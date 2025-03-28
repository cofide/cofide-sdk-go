// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package xds

import (
	"context"
	"log/slog"
	"net"
	"testing"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestXDSClient_NewXDSClient(t *testing.T) {
	cfg := XDSClientConfig{
		ServerURI: "test-server",
		NodeID:    "test-client",
	}

	client, err := NewXDSClient(cfg)
	assert.NotNil(t, client)
	assert.NoError(t, err)
	assert.Equal(t, client.nodeID, cfg.NodeID)
	assert.NotNil(t, client.client)
}

func TestXDSClient_XDSComms(t *testing.T) {
	client, lis, mocked := setupBufconn()
	defer lis.Close()

	client.watchEndpoints(context.Background(), "test")
	assert.True(t, mocked.Called)
}

type MockAggregatedDiscoveryService struct {
	discoveryv3.UnimplementedAggregatedDiscoveryServiceServer
	Called bool
}

func (m *MockAggregatedDiscoveryService) StreamAggregatedResources(
	stream discoveryv3.AggregatedDiscoveryService_StreamAggregatedResourcesServer,
) error {
	m.Called = true
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
