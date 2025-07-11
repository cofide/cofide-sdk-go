// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package xds

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"

	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type XDSClient struct {
	conn      *grpc.ClientConn
	client    discovery.AggregatedDiscoveryServiceClient
	nodeID    string
	endpoints sync.Map // service -> []Endpoint
	watching  sync.Map // service -> *sync.Once
}

type XDSClientConfig struct {
	ServerURI string
	NodeID    string
}

type Endpoint struct {
	Host   string
	Port   int
	Weight int
}

func NewXDSClient(cfg XDSClientConfig) (*XDSClient, error) {
	conn, err := grpc.NewClient(
		cfg.ServerURI,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // insecure connection
	)
	if err != nil {
		return nil, err
	}

	client := &XDSClient{
		conn:   conn,
		client: discovery.NewAggregatedDiscoveryServiceClient(conn),
		nodeID: cfg.NodeID,
	}

	return client, nil
}

func (c *XDSClient) watchEndpoints(ctx context.Context, serviceName string) {
	// Clusters in Cofide Agent xDS have a _cluster suffix
	xdsResourceName := fmt.Sprintf("%v_cluster", serviceName)

	logger := slog.With(
		slog.String("service", serviceName),
		slog.String("cluster", xdsResourceName),
		slog.String("node", c.nodeID),
	)

	stream, err := c.client.StreamAggregatedResources(ctx)
	if err != nil {
		logger.Error("Failed to create xDS stream", "error", err)
		return
	}

	defer func() {
		if err := stream.CloseSend(); err != nil {
			logger.Error("Error closing xDS stream", "error", err)
		}
	}()

	// Send EDS request
	req := &discovery.DiscoveryRequest{
		Node: &core.Node{
			Id: c.nodeID,
		},
		TypeUrl:       resource.EndpointType, // Type URL for endpoints
		ResourceNames: []string{xdsResourceName},
	}
	if err := stream.Send(req); err != nil {
		logger.Error("Failed to send xDS discovery request", "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug("xDS watch complete")
			return
		default:
			resp, err := stream.Recv()
			if err != nil {
				logger.Error("Failed to receive xDS discovery response", "error", err)
				return
			}

			// Update endpoints directly in cache
			if len(resp.Resources) > 0 {
				var cla endpoint.ClusterLoadAssignment
				if err := resp.Resources[0].UnmarshalTo(&cla); err != nil {
					logger.Error("Failed to unmarshal ClusterLoadAssignment", "error", err)
					continue
				}

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
				c.endpoints.Store(serviceName, endpoints)
				logger.Debug("Endpoints updated", slog.Any("endpoints", endpoints))
			} else {
				logger.Debug("No endpoints in xDS response")
			}
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
		go c.watchEndpoints(context.Background(), service)
	})

	// Return empty for now, next request will get the endpoints
	return nil, fmt.Errorf("endpoints not yet discovered for %s", service)
}
