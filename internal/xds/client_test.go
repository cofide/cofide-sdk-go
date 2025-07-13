// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package xds

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
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
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestXDSClient_NewXDSClient(t *testing.T) {
	cfg := XDSClientConfig{
		Logger:    slog.Default(),
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

func TestXDSClient_GetEndpoints(t *testing.T) {
	client, lis, mocked := setupBufconn(t)
	defer lis.Close()

	// First call to GetEndpoints starts watchEndpoints.
	_, err := client.GetEndpoints("test-service")
	require.Error(t, err)
	assert.ErrorContains(t, err, "endpoints not yet discovered for test-service")

	// Response has a single endpoint.
	endpoints := []Endpoint{{Host: "1.2.3.4", Port: 4321, Weight: 42}}
	cla, err := makeCLA(endpoints)
	require.NoError(t, err)

	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{cla}})

	assertEndpoints(t, client, endpoints)

	require.NotEmpty(t, mocked.reqs)
	assert.EqualExportedValues(t, &core.Node{Id: "test-client"}, mocked.reqs[0].Node)
	assert.EqualExportedValues(t, resource.EndpointType, mocked.reqs[0].TypeUrl)
	assert.EqualExportedValues(t, []string{"test-service_cluster"}, mocked.reqs[0].ResourceNames)
}

func TestXDSClient_GetEndpoints_update(t *testing.T) {
	client, lis, mocked := setupBufconn(t)
	defer lis.Close()

	// First call to GetEndpoints starts watchEndpoints.
	_, err := client.GetEndpoints("test-service")
	require.Error(t, err)
	assert.ErrorContains(t, err, "endpoints not yet discovered for test-service")

	// First response has a single endpoint.
	endpoints := []Endpoint{{Host: "1.2.3.4", Port: 4321, Weight: 42}}
	cla, err := makeCLA(endpoints)
	require.NoError(t, err)

	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{cla}})

	assertEndpoints(t, client, endpoints)

	// Second response adds a second endpoint.
	endpoints = []Endpoint{
		{Host: "1.2.3.4", Port: 4321, Weight: 42},
		{Host: "1.2.3.5", Port: 4322, Weight: 43},
	}
	cla, err = makeCLA(endpoints)
	require.NoError(t, err)

	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{cla}})

	assertEndpoints(t, client, endpoints)
	time.Sleep(1 * time.Second)
}

func TestXDSClient_GetEndpoints_noEndpoints(t *testing.T) {
	client, lis, mocked := setupBufconn(t)
	defer lis.Close()

	// First call to GetEndpoints starts watchEndpoints.
	_, err := client.GetEndpoints("test-service")
	require.Error(t, err)
	assert.ErrorContains(t, err, "endpoints not yet discovered for test-service")

	// Response has a single endpoint.
	endpoints := []Endpoint{{Host: "1.2.3.4", Port: 4321, Weight: 42}}
	cla, err := makeCLA(endpoints)
	require.NoError(t, err)

	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{cla}})

	assertEndpoints(t, client, endpoints)

	// Send a response with no endpoints
	emptyCLA, err := makeCLA(nil)
	require.NoError(t, err)
	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{emptyCLA}})

	// Endpoints should be removed from cache.
	assertEndpoints(t, client, []Endpoint{})

	// Replace endpoints in response.
	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{cla}})

	// Endpoints should be replaced in cache.
	assertEndpoints(t, client, endpoints)
}

func TestXDSClient_GetEndpoints_noResources(t *testing.T) {
	client, lis, mocked := setupBufconn(t)
	defer lis.Close()

	// First call to GetEndpoints starts watchEndpoints.
	_, err := client.GetEndpoints("test-service")
	require.Error(t, err)
	assert.ErrorContains(t, err, "endpoints not yet discovered for test-service")

	// Response has a single endpoint.
	endpoints := []Endpoint{{Host: "1.2.3.4", Port: 4321, Weight: 42}}
	cla, err := makeCLA(endpoints)
	require.NoError(t, err)

	// Initial response has no resources.
	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{}})
	// Second response has an endpoint.
	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{cla}})

	assertEndpoints(t, client, endpoints)

	// Send a response with no resources.
	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{}})

	// Endpoints should be removed from cache.
	assertEndpoints(t, client, []Endpoint{})

	// Replace resources in response.
	cla, err = makeCLA(endpoints)
	require.NoError(t, err)
	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{cla}})

	// Endpoints should be replaced in cache.
	assertEndpoints(t, client, endpoints)
}

func TestXDSClient_GetEndpoints_sendError(t *testing.T) {
	client, lis, mocked := setupBufconn(t)
	defer lis.Close()

	// First call to GetEndpoints starts watchEndpoints.
	_, err := client.GetEndpoints("test-service")
	require.Error(t, err)
	assert.ErrorContains(t, err, "endpoints not yet discovered for test-service")

	// First client call to Send returns an error.
	// The channel is blocking, so we know this will be consumed before the response.
	mocked.error(errors.New("failed to open stream"))

	// Then respond with a single endpoint.
	endpoints := []Endpoint{{Host: "1.2.3.4", Port: 4321, Weight: 42}}
	cla, err := makeCLA(endpoints)
	require.NoError(t, err)

	mocked.respond(&discovery.DiscoveryResponse{Resources: []*anypb.Any{cla}})

	assertEndpoints(t, client, endpoints)
}

// makeCLA returns a ClusterLoadAssignment for a slice of Endpoint, encoded as an anypb.Any.
func makeCLA(endpoints []Endpoint) (*anypb.Any, error) {
	localityEps := []*endpoint.LocalityLbEndpoints{}
	for _, ep := range endpoints {
		localityEps = append(localityEps, &endpoint.LocalityLbEndpoints{
			LbEndpoints: []*endpoint.LbEndpoint{
				{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: &core.Address{
								Address: &core.Address_SocketAddress{
									SocketAddress: &core.SocketAddress{
										Address: ep.Host,
										PortSpecifier: &core.SocketAddress_PortValue{
											PortValue: uint32(ep.Port),
										},
									},
								},
							},
						},
					},
					LoadBalancingWeight: &wrapperspb.UInt32Value{
						Value: uint32(ep.Weight),
					},
				},
			},
		})
	}
	return anypb.New(&endpoint.ClusterLoadAssignment{Endpoints: localityEps})
}

// assertEndpoints asserts that the client eventually returns the specified endpoints from GetEndpoints.
func assertEndpoints(t *testing.T, client *XDSClient, endpoints []Endpoint) {
	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		got, err := client.GetEndpoints("test-service")
		require.NoError(collect, err)
		assert.Equal(collect, endpoints, got)
	}, 10*time.Second, 100*time.Millisecond)
}

type MockAggregatedDiscoveryService struct {
	discoveryv3.UnimplementedAggregatedDiscoveryServiceServer
	t      *testing.T
	reqs   []*discovery.DiscoveryRequest
	respCh chan *discovery.DiscoveryResponse
	errCh  chan error
}

func newMockAggregatedDiscoveryService(t *testing.T) *MockAggregatedDiscoveryService {
	return &MockAggregatedDiscoveryService{
		t:      t,
		respCh: make(chan *discoveryv3.DiscoveryResponse),
		errCh:  make(chan error),
	}
}

func (m *MockAggregatedDiscoveryService) StreamAggregatedResources(
	stream discoveryv3.AggregatedDiscoveryService_StreamAggregatedResourcesServer,
) error {
	for {
		// Wait for a DiscoveryRequest
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		m.reqs = append(m.reqs, req)

		// Wait for the test harness to send us either a response or error to return to the client.
		select {
		case resp, ok := <-m.respCh:
			if !ok {
				return nil
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
		case err := <-m.errCh:
			return err
		}
	}
}

// respond sends a response on respCh for the server to send to the client.
func (m *MockAggregatedDiscoveryService) respond(resp *discovery.DiscoveryResponse) {
	select {
	case m.respCh <- resp:
	case <-time.After(5 * time.Second):
		m.t.Fatal("Timed out waiting to send to response channel")
	}
}

// error sends an error on errCh for the server to send to the client.
func (m *MockAggregatedDiscoveryService) error(err error) {
	select {
	case m.errCh <- err:
	case <-time.After(5 * time.Second):
		m.t.Fatal("Timed out waiting to send to error channel")
	}
}

// setupBufconn creates a bufconn-enabled grpc server with a mock ADS implementation
// for unit test usage when testing the cofide-sdk-go xDS functionality
func setupBufconn(t *testing.T, opts ...grpc.DialOption) (*XDSClient, *bufconn.Listener, *MockAggregatedDiscoveryService) {
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	mockADSService := newMockAggregatedDiscoveryService(t)
	discoveryv3.RegisterAggregatedDiscoveryServiceServer(srv, mockADSService)

	go func() {
		_ = srv.Serve(lis)
	}()

	cfg := XDSClientConfig{
		Logger: makeLogger(),
		// NB: passthrough is required to avoid dns resolution
		ServerURI: "passthrough:///test-server",
		NodeID:    "test-client",
	}

	if len(opts) == 0 {
		opts = []grpc.DialOption{grpc.WithContextDialer(
			func(context.Context, string) (net.Conn, error) {
				return lis.Dial()
			},
		)}
	}
	client, err := NewXDSClient(cfg, opts...)
	require.NoError(t, err)

	slog.Debug("running bufconn test server", "target", client.conn.Target())

	return client, lis, mockADSService
}

// makeLogger returns a Logger that sends logs to stderr.
func makeLogger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}
