// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package xds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/cofide/cofide-sdk-go/internal/backoff"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"

	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type XDSClient struct {
	logger    *slog.Logger
	conn      *grpc.ClientConn
	client    discovery.AggregatedDiscoveryServiceClient
	nodeID    string
	endpoints sync.Map // service -> []Endpoint
	watching  sync.Map // service -> *sync.Once
}

type XDSClientConfig struct {
	Logger    *slog.Logger
	ServerURI string
	NodeID    string
}

type Endpoint struct {
	Host   string
	Port   int
	Weight int
}

func NewXDSClient(cfg XDSClientConfig, opts ...grpc.DialOption) (*XDSClient, error) {
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials())) // insecure connection
	conn, err := grpc.NewClient(
		cfg.ServerURI,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	client := &XDSClient{
		logger: cfg.Logger.With(slog.String("node", cfg.NodeID)),
		conn:   conn,
		client: discovery.NewAggregatedDiscoveryServiceClient(conn),
		nodeID: cfg.NodeID,
	}

	return client, nil
}

func (c *XDSClient) watchEndpointsRetried(ctx context.Context, serviceName string) {
	logger := c.logger.With(slog.String("service", serviceName))
	backoff := backoff.NewBackoff()
	for {
		resetBackoff, err := c.watchEndpoints(ctx, logger, serviceName)
		if err != nil {
			logger.Error("xDS watch failed, retrying", "error", err)
		}
		if resetBackoff {
			backoff.Reset()
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff.Duration()):
		}
	}
}

// watchEndpoints watches endpoints for a service using an ADS stream.
// The endpoints map is updated with the current state of the endpoints.
// watchEndpoints returns if the stream is closed or any send/receive request fails.
// It returns a bool indicating whether the backoff in the caller should be reset, as well as an error.
func (c *XDSClient) watchEndpoints(ctx context.Context, logger *slog.Logger, serviceName string) (bool, error) {
	// Clusters in Cofide Agent xDS have a _cluster suffix
	xdsResourceName := fmt.Sprintf("%v_cluster", serviceName)

	logger.Debug("Connecting to xDS server")
	stream, err := c.client.StreamAggregatedResources(ctx)
	if err != nil {
		return false, fmt.Errorf("Failed to create xDS stream: %w", err)
	}

	defer func() {
		if err := stream.CloseSend(); err != nil {
			logger.Error("Error closing xDS stream", "error", err)
		}
	}()

	req := &discovery.DiscoveryRequest{
		Node: &core.Node{
			Id: c.nodeID,
		},
		TypeUrl:       resource.EndpointType, // Type URL for endpoints
		ResourceNames: []string{xdsResourceName},
	}

	// resetBackoff tracks whether we have seen a valid endpoint, and should reset the backoff.
	var resetBackoff bool
	for {
		// Send EDS request
		if err := stream.Send(req); err != nil {
			return resetBackoff, fmt.Errorf("failed to send xDS discovery request: %w", err)
		}

		logger.Debug("Sent xDS discovery request")

		select {
		case <-ctx.Done():
			logger.Debug("xDS watch cancelled")
			return resetBackoff, nil
		default:
			resp, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					logger.Debug("xDS watch stream ended")
					resetBackoff = true
				} else {
					err = fmt.Errorf("failed to receive xDS discovery response: %w", err)
				}
				return resetBackoff, err
			}

			resetBackoff = true

			// Update the last seen version and nonce in the request.
			req.VersionInfo = resp.VersionInfo
			req.ResponseNonce = resp.Nonce

			// Update endpoints directly in cache
			endpoints := []Endpoint{}
			if len(resp.Resources) > 0 {
				var cla endpoint.ClusterLoadAssignment
				if err := resp.Resources[0].UnmarshalTo(&cla); err != nil {
					logger.Error("Failed to unmarshal ClusterLoadAssignment", "error", err)
					continue
				} else {
					endpoints = claToEndpoints(&cla)
					logger.Debug("xDS endpoints updated", slog.Any("endpoints", endpoints))
				}
			} else {
				logger.Debug("No endpoints in xDS response")
			}
			c.endpoints.Store(serviceName, endpoints)
		}
	}
}

func (c *XDSClient) GetEndpoints(service string) ([]Endpoint, error) {
	// First check if we already have endpoints
	if eps, ok := c.endpoints.Load(service); ok {
		return eps.([]Endpoint), nil
	}

	// Check if we're already watching, using sync.Once per service
	watchOnce, _ := c.watching.LoadOrStore(service, &sync.Once{})
	watchOnce.(*sync.Once).Do(func() {
		go c.watchEndpointsRetried(context.Background(), service)
	})

	// Return empty for now, next request will get the endpoints
	return nil, fmt.Errorf("endpoints not yet discovered for %s", service)
}

// claToEndpoints converts a ClusterLoadAssignment to a slice of Endpoint.
func claToEndpoints(cla *endpoint.ClusterLoadAssignment) []Endpoint {
	endpoints := make([]Endpoint, 0)

	for _, locality := range cla.Endpoints {
		for _, endpoint := range locality.LbEndpoints {
			addr := endpoint.GetEndpoint().Address.GetSocketAddress()
			endpoints = append(endpoints, Endpoint{
				Host:   addr.GetAddress(),
				Port:   int(addr.GetPortValue()),
				Weight: int(endpoint.GetLoadBalancingWeight().GetValue()),
			})
		}
	}
	return endpoints
}
