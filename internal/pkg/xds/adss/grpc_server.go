// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adss

import (
	"context"
	"fmt"
	"net"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/jewertow/federation/internal/pkg/xds"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
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
		log.Infof("[%s] Running gRPC server", s.opts.ServerID)
		return s.grpc.Serve(listener)
	})

	routinesGroup.Go(func() error {
		defer log.Infof("[%s] gRPC server was shut down", s.opts.ServerID)
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
			log.Infof("[%s] Received push request: %v", s.opts.ServerID, pushRequest)
			if err := s.ads.push(pushRequest); err != nil {
				log.Errorf("[%s] failed to push to subscribers: %v", s.opts.ServerID, err)
			}
		}
	}

	return routinesGroup.Wait()
}
