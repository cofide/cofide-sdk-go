package xds

import (
	"context"
	"fmt"
	"log/slog"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"

	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type XDSClient struct {
	conn   *grpc.ClientConn
	client discovery.AggregatedDiscoveryServiceClient
	nodeID string
}

type XDSClientConfig struct {
	ServerURI string
	Creds     grpc.DialOption
	NodeID    string
}

func NewXDSClient(ctx context.Context, cfg XDSClientConfig) (*XDSClient, error) {
	conn, err := grpc.NewClient(
		cfg.ServerURI,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // insecure connection
	)
	if err != nil {
		return nil, err
	}

	return &XDSClient{
		conn:   conn,
		client: discovery.NewAggregatedDiscoveryServiceClient(conn),
		nodeID: cfg.NodeID,
	}, nil
}

func (c *XDSClient) GetClusters() ([]string, error) {
	ctx := context.Background()

	// Create ADS stream
	stream, err := c.client.StreamAggregatedResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %v", err)
	}

	defer func() {
		if err := stream.CloseSend(); err != nil {
			slog.Error("error closing stream", "error", err)
		}
	}()

	// Send CDS request
	req := &discovery.DiscoveryRequest{
		Node: &core.Node{
			Id: c.nodeID,
		},
		TypeUrl: resource.ClusterType, // Type URL for clusters
	}
	if err := stream.Send(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	// Wait for response
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive response: %v", err)
	}

	// Extract cluster names
	var clusterNames []string
	for _, res := range resp.Resources {
		var cluster cluster.Cluster
		if err := anypb.UnmarshalTo(res, &cluster, proto.UnmarshalOptions{}); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cluster: %v", err)
		}
		clusterNames = append(clusterNames, cluster.Name)
	}

	return clusterNames, nil
}

func (c *XDSClient) GetClusterEndpoints() (map[string]*endpoint.ClusterLoadAssignment, error) {
	// Implementation to fetch endpoints via xDS
	ctx := context.Background()

	// Create ADS stream
	stream, err := c.client.StreamAggregatedResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %v", err)
	}

	defer func() {
		if err := stream.CloseSend(); err != nil {
			slog.Error("error closing stream", "error", err)
		}
	}()

	// Send EDS request
	req := &discovery.DiscoveryRequest{
		Node: &core.Node{
			Id: c.nodeID,
		},
		TypeUrl: resource.EndpointType, // Type URL for endpoints
	}
	if err := stream.Send(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	// Wait for response
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive response: %v", err)
	}

	clas := make(map[string]*endpoint.ClusterLoadAssignment)
	for _, res := range resp.Resources {
		cla := &endpoint.ClusterLoadAssignment{}
		if err := res.UnmarshalTo(cla); err != nil {
			return nil, fmt.Errorf("failed to unmarshal endpoint: %v", err)
		}
		clas[cla.ClusterName] = cla
	}

	return clas, nil
}
