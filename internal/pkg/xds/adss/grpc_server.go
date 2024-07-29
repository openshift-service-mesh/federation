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

type Server struct {
	grpc         *grpc.Server
	ads          *adsServer
	pushRequests <-chan xds.PushRequest
	port         int32
	serverID     string
}

func NewServer(pushRequests <-chan xds.PushRequest, generators []xds.ResourceGenerator, port int32, serverID string) *Server {
	grpcServer := grpc.NewServer()
	generatorsMap := make(map[string]xds.ResourceGenerator)
	for _, g := range generators {
		generatorsMap[g.GetTypeUrl()] = g
	}
	adsServer := &adsServer{generators: generatorsMap, serverID: serverID}

	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, adsServer)

	return &Server{
		grpc:         grpcServer,
		ads:          adsServer,
		pushRequests: pushRequests,
		port:         port,
		serverID:     serverID,
	}
}

// Run starts the gRPC server and the controllers.
func (s *Server) Run(ctx context.Context) error {
	var routinesGroup errgroup.Group

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("[%s] creating TCP listener: %w", s.serverID, err)
	}

	ctx, cancel := context.WithCancel(ctx)

	routinesGroup.Go(func() error {
		defer cancel()
		klog.Infof("[%s] Running gRPC server", s.serverID)
		return s.grpc.Serve(listener)
	})

	routinesGroup.Go(func() error {
		defer klog.Info("[%s] gRPC server was shut down", s.serverID)
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

		case pushRequest := <-s.pushRequests:
			klog.Infof("[%s] Received push request: %v", s.serverID, pushRequest)
			if err := s.ads.push(pushRequest); err != nil {
				klog.Errorf("[%s] failed to push to subscribers: %v", s.serverID, err)
			}
		}
	}

	return routinesGroup.Wait()
}
