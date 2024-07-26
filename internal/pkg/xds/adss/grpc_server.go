package adss

import (
	"context"
	"fmt"
	"net"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/jewertow/federation/internal/pkg/xds"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

// defaultServerPort is the default port for the gRPC server.
const defaultServerPort string = "15010"

type Server struct {
	grpc         *grpc.Server
	ads          *adsServer
	pushRequests <-chan xds.PushRequest
}

func NewServer(pushRequests <-chan xds.PushRequest, generators []xds.ResourceGenerator) *Server {
	grpcServer := grpc.NewServer()
	generatorsMap := make(map[string]xds.ResourceGenerator)
	for _, g := range generators {
		generatorsMap[g.GetTypeUrl()] = g
	}
	adsServer := &adsServer{generators: generatorsMap}

	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, adsServer)

	return &Server{
		grpc:         grpcServer,
		ads:          adsServer,
		pushRequests: pushRequests,
	}
}

// Run starts the gRPC server and the controllers.
func (s *Server) Run(ctx context.Context) error {
	var routinesGroup errgroup.Group

	listener, err := net.Listen("tcp", fmt.Sprint(":", defaultServerPort))
	if err != nil {
		return fmt.Errorf("creating TCP listener: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	routinesGroup.Go(func() error {
		defer cancel()
		klog.Info("Running MCP RPC server")
		return s.grpc.Serve(listener)
	})

	routinesGroup.Go(func() error {
		defer klog.Info("MCP gRPC server was shut down")
		<-ctx.Done()
		s.grpc.GracefulStop()
		return nil
	})

loop:
	for {
		select {
		case <-ctx.Done():
			s.ads.closeSubscribers()
			break loop

		case mcpEvent := <-s.pushRequests:
			if err := s.ads.push(mcpEvent); err != nil {
				klog.Errorf("Error pushing to subscribers: %v", err)
			}
		}
	}

	return routinesGroup.Wait()
}
