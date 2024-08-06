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
	opts         *ServerOpts
}

type ServerOpts struct {
	Port     int32
	ServerID string
}

func NewServer(opts *ServerOpts, pushRequests <-chan xds.PushRequest, onNewSubscriber func(), handlers ...RequestHandler) *Server {
	// TODO: handle nil opts
	grpcServer := grpc.NewServer()
	handlerMap := make(map[string]RequestHandler)
	for _, g := range handlers {
		handlerMap[g.GetTypeUrl()] = g
	}
	ads := &adsServer{
		handlers:        handlerMap,
		onNewSubscriber: onNewSubscriber,
		serverID:        opts.ServerID,
	}

	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, ads)

	return &Server{
		grpc:         grpcServer,
		ads:          ads,
		pushRequests: pushRequests,
		opts:         opts,
	}
}

// Run starts the gRPC server and the controllers.
func (s *Server) Run(ctx context.Context) error {
	var routinesGroup errgroup.Group

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.opts.Port))
	if err != nil {
		return fmt.Errorf("[%s] creating TCP listener: %w", s.opts.ServerID, err)
	}

	ctx, cancel := context.WithCancel(ctx)

	routinesGroup.Go(func() error {
		defer cancel()
		klog.Infof("[%s] Running gRPC server", s.opts.ServerID)
		return s.grpc.Serve(listener)
	})

	routinesGroup.Go(func() error {
		defer klog.Info("[%s] gRPC server was shut down", s.opts.ServerID)
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
			klog.Infof("[%s] Received push request: %v", s.opts.ServerID, pushRequest)
			if err := s.ads.push(pushRequest); err != nil {
				klog.Errorf("[%s] failed to push to subscribers: %v", s.opts.ServerID, err)
			}
		}
	}

	return routinesGroup.Wait()
}
