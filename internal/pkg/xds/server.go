package xds

import (
	"context"
	"fmt"
	"net"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/jewertow/federation/internal/pkg/mcp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

// defaultServerPort is the default port for the gRPC server.
const defaultServerPort string = "15010"

// Run starts the gRPC server and the controllers.
func Run(ctx context.Context, pushMCP <-chan mcp.McpResources) error {
	var routinesGroup errgroup.Group
	grpcServer := grpc.NewServer()
	adsServerImpl := &adsServer{}

	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, adsServerImpl)

	listener, err := net.Listen("tcp", fmt.Sprint(":", defaultServerPort))
	if err != nil {
		return fmt.Errorf("creating TCP listener: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	routinesGroup.Go(func() error {
		defer cancel()
		klog.Info("Running MCP RPC server")
		return grpcServer.Serve(listener)
	})

	routinesGroup.Go(func() error {
		defer klog.Info("MCP gRPC server was shut down")
		<-ctx.Done()
		grpcServer.GracefulStop()
		return nil
	})

loop:
	for {
		select {
		case <-ctx.Done():
			adsServerImpl.closeSubscribers()
			break loop

		case mcpResources := <-pushMCP:
			klog.Infof("Pushing MCP resources to subscribers: %v", mcpResources)
			if err := adsServerImpl.push(mcpResources); err != nil {
				klog.Errorf("Error pushing to subscribers: %v", err)
			}
		}
	}

	return routinesGroup.Wait()
}
