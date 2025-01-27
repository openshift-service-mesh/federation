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

package discovery

import (
	"context"
	"fmt"
	"net"
	"time"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc"

	"github.com/openshift-service-mesh/federation/internal/pkg/legacy/xds"
)

const restartDelay = 2 * time.Second

type Server struct {
	grpc    *grpc.Server
	ads     *adsServer
	running bool
}

func NewServer(handlers ...RequestHandler) *Server {
	grpcServer := grpc.NewServer()
	handlerMap := make(map[string]RequestHandler)
	for _, g := range handlers {
		handlerMap[g.GetTypeUrl()] = g
	}
	ads := &adsServer{
		handlers: handlerMap,
	}

	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, ads)

	return &Server{
		grpc: grpcServer,
		ads:  ads,
	}
}

func (s *Server) PushAll(pushRequest xds.PushRequest) error {
	return s.ads.Push(pushRequest)
}

// Run starts the gRPC server and awaits for push requests to broadcast configuration.
func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", ":15080")
	if err != nil {
		return fmt.Errorf("failed creating TCP listener: %w", err)
	}

	// TODO(fdsserver): rethink how to handle graceful stop
	defer s.Stop()

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		s.running = true // TODO(fdsserver): is that safe enough to assume?
		if err := s.grpc.Serve(listener); err != nil {
			cancel()
		}
	}()

	<-ctx.Done()

	return nil
}

func (s *Server) IsRunning() bool {
	return s.running
}

// StartOnce will start the server if it's not already running.
// Returns true if it has been started or false if it's been already running.
func (s *Server) StartOnce(ctx context.Context) bool {
	if !s.IsRunning() {
		go func() {
			for {
				if err := s.Run(ctx); err != nil {
					log.Errorf("server encountered an error: %v, restarting in %s...", err, restartDelay)

					select {
					case <-time.After(restartDelay):
					case <-ctx.Done():
						log.Info("context canceled, stopping restart attempts")
						return
					}
				} else {
					break // running, break out of the retry loop
				}
			}
		}()

		return true
	}

	return false
}

func (s *Server) Stop() {
	s.ads.closeSubscribers()
	s.grpc.GracefulStop()
	s.running = false
}
