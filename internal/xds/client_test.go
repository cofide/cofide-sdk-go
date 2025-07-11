// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package xds

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestXDSClient_NewXDSClient(t *testing.T) {
	cfg := XDSClientConfig{
		ServerURI: "test-server:4321",
		NodeID:    "test-client",
	}

	client, err := NewXDSClient(cfg)
	assert.NotNil(t, client)
	assert.NoError(t, err)
	assert.Equal(t, client.nodeID, cfg.NodeID)
	assert.NotNil(t, client.client)
	assert.Equal(t, "dns:///test-server:4321", client.conn.CanonicalTarget())
}

func TestXDSClient_XDSComms(t *testing.T) {
	client, lis, mocked := setupBufconn()
	defer lis.Close()

	// First response is empty.
	cla1, err := anypb.New(&endpoint.ClusterLoadAssignment{
		Endpoints: []*endpoint.LocalityLbEndpoints{},
	})
	require.NoError(t, err)
	// Second response has a single endpoint.
	cla2, err := anypb.New(&endpoint.ClusterLoadAssignment{
		Endpoints: []*endpoint.LocalityLbEndpoints{
			{
				LbEndpoints: []*endpoint.LbEndpoint{
					{
						HostIdentifier: &endpoint.LbEndpoint_Endpoint{
							Endpoint: &endpoint.Endpoint{
								Address: &core.Address{
									Address: &core.Address_SocketAddress{
										SocketAddress: &core.SocketAddress{
											Address: "1.2.3.4",
											PortSpecifier: &core.SocketAddress_PortValue{
												PortValue: 4321,
											},
										},
									},
								},
							},
						},
						LoadBalancingWeight: &wrapperspb.UInt32Value{
							Value: 42,
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	mocked.resps = []*discovery.DiscoveryResponse{
		{Resources: []*anypb.Any{cla1}},
		{Resources: []*anypb.Any{cla2}},
	}

	mocked.errs = []error{errors.New("no spoons"), errors.New("lost my wallet")}

	_, err = client.GetEndpoints("test-service")
	require.Error(t, err)
	assert.ErrorContains(t, err, "endpoints not yet discovered for test-service")

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		endpoints, err := client.GetEndpoints("test-service")
		require.NoError(collect, err)
		expected := []Endpoint{{Host: "1.2.3.4", Port: 4321, Weight: 42}}
		assert.Equal(collect, expected, endpoints)
	}, 10*time.Second, 100*time.Millisecond)

	assert.NotNil(t, mocked.req)
	assert.EqualExportedValues(t, &core.Node{Id: "test-client"}, mocked.req.Node)
	assert.EqualExportedValues(t, resource.EndpointType, mocked.req.TypeUrl)
	assert.EqualExportedValues(t, []string{"test-service_cluster"}, mocked.req.ResourceNames)

}

type MockAggregatedDiscoveryService struct {
	discoveryv3.UnimplementedAggregatedDiscoveryServiceServer
	req   *discovery.DiscoveryRequest
	resps []*discovery.DiscoveryResponse
	errs  []error
}

func (m *MockAggregatedDiscoveryService) StreamAggregatedResources(
	stream discoveryv3.AggregatedDiscoveryService_StreamAggregatedResourcesServer,
) error {
	// Allow injection of errors
	if len(m.errs) > 0 {
		err := m.errs[0]
		m.errs = m.errs[1:]
		return err
	}

	req, err := stream.Recv()
	if err != nil {
		return err
	}

	// Record the request
	m.req = req

	// Stream each response in turn
	for _, resp := range m.resps {
		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	return nil
}

// setupBufconn creates a bufconn-enabled grpc server with a mock ADS implementation
// for unit test usage when testing the cofide-sdk-go xDS functionality
func setupBufconn() (*XDSClient, *bufconn.Listener, *MockAggregatedDiscoveryService) {
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	mockADSService := &MockAggregatedDiscoveryService{}
	discoveryv3.RegisterAggregatedDiscoveryServiceServer(srv, mockADSService)

	go func() {
		if err := srv.Serve(lis); err != nil {
			panic(err)
		}
	}()

	cfg := XDSClientConfig{
		// NB: passthrough is required to avoid dns resolution
		ServerURI: "passthrough:///test-server",
		NodeID:    "test-client",
	}

	conn, _ := grpc.NewClient(
		cfg.ServerURI,
		grpc.WithContextDialer(
			func(context.Context, string) (net.Conn, error) {
				return lis.Dial()
			},
		),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	client := &XDSClient{
		conn:   conn,
		client: discovery.NewAggregatedDiscoveryServiceClient(conn),
		nodeID: cfg.NodeID,
	}

	slog.Info("running bufconn test server", "target", conn.Target())

	return client, lis, mockADSService
}
