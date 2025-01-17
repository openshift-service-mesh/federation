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
	"google.golang.org/grpc"

	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

type Server struct {
	grpc         *grpc.Server
	ads          *adsServer
	pushRequests <-chan xds.PushRequest
}

func NewServer(pushRequests <-chan xds.PushRequest, onNewSubscriber func(), handlers ...RequestHandler) *Server {
	grpcServer := grpc.NewServer()
	handlerMap := make(map[string]RequestHandler)
	for _, g := range handlers {
		handlerMap[g.GetTypeUrl()] = g
	}
	ads := &adsServer{
		handlers:        handlerMap,
		onNewSubscriber: onNewSubscriber,
	}

	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, ads)

	return &Server{
		grpc:         grpcServer,
		ads:          ads,
		pushRequests: pushRequests,
	}
}

// Run starts the gRPC server and awaits for push requests to broadcast configuration.
func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", ":15080")
	if err != nil {
		return fmt.Errorf("creating TCP listener: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		log.Info("Running gRPC server")
		if err := s.grpc.Serve(listener); err != nil {
			cancel()
		}
	}()

loop:
	for {
		select {
		case <-ctx.Done():
			s.ads.closeSubscribers()
			s.grpc.GracefulStop()
			log.Info("gRPC server was shut down")
			break loop

		case pushRequest := <-s.pushRequests:
			log.Infof("Received push request: %v", pushRequest)
			if err := s.ads.push(pushRequest); err != nil {
				log.Errorf("failed to push to subscribers: %v", err)
			}
		}
	}

	return nil
}
