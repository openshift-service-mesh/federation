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

package adsc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	istiolog "istio.io/istio/pkg/log"
)

const (
	defaultClientMaxReceiveMessageSize = math.MaxInt32
	defaultInitialConnWindowSize       = 1024 * 1024 // default gRPC InitialWindowSize
	defaultInitialWindowSize           = 1024 * 1024 // default gRPC ConnWindowSize
)

type ADSCConfig struct {
	RemoteName     string
	DiscoveryAddr  string
	Authority      string
	Handlers       map[string]ResponseHandler
	ReconnectDelay time.Duration
}

type ADSC struct {
	stream discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient
	conn   *grpc.ClientConn
	cfg    *ADSCConfig
	log    *istiolog.Scope
}

func New(opts *ADSCConfig) (*ADSC, error) {
	if opts == nil {
		return nil, errors.New("adsc: opts is nil")
	}
	adsc := &ADSC{
		cfg: opts,
		log: istiolog.RegisterScope("adsc", "Aggregated Discovery Service Client").WithLabels("peer", opts.RemoteName),
	}
	if err := adsc.dial(); err != nil {
		return nil, err
	}

	return adsc, nil
}

func (a *ADSC) Run(ctx context.Context) error {
	client := discovery.NewAggregatedDiscoveryServiceClient(a.conn)

	var err error
	if a.stream, err = client.StreamAggregatedResources(ctx); err != nil {
		return fmt.Errorf("failed setting resource stream: %w", err)
	}

	for k, _ := range a.cfg.Handlers {
		discoveryRequest := &discovery.DiscoveryRequest{TypeUrl: k}
		if errSend := a.Send(discoveryRequest); errSend != nil {
			a.log.Errorf("[%s] failed requesting initial discovery sync: %+v", k, errSend)
		}
	}

	go a.handleRecv(ctx)

	return nil
}

func (a *ADSC) Restart(ctx context.Context) {
	a.log.Infof("reconnecting to ADS server %s", a.cfg.DiscoveryAddr)
	if err := a.Run(ctx); err != nil {
		a.log.Errorf("failed to connect to ADS server %s, will reconnect in %s: %v", a.cfg.DiscoveryAddr, a.cfg.ReconnectDelay, err)
		time.AfterFunc(a.cfg.ReconnectDelay, func() {
			if errCtx := ctx.Err(); errCtx != nil {
				a.log.Infof("Parent ctx is done: %v", errCtx)
				return
			}

			a.Restart(ctx)
		})
	}
}

func (a *ADSC) Send(req *discovery.DiscoveryRequest) error {
	req.ResponseNonce = time.Now().String()
	a.log.Infof("Sending Discovery Request to ADS server: %s", req.String())
	return a.stream.Send(req)
}

func (a *ADSC) dial() error {
	backoffConfig := backoff.DefaultConfig
	backoffConfig.MaxDelay = a.cfg.ReconnectDelay

	var err error
	a.conn, err = grpc.NewClient(
		a.cfg.DiscoveryAddr,
		grpc.WithAuthority(a.cfg.Authority),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithInitialWindowSize(int32(defaultInitialWindowSize)),
		grpc.WithInitialConnWindowSize(int32(defaultInitialConnWindowSize)),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(defaultClientMaxReceiveMessageSize)),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: a.cfg.ReconnectDelay,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to establish connection to the ADS server %s: %w", a.cfg.DiscoveryAddr, err)
	}
	return nil
}

func (a *ADSC) handleRecv(ctx context.Context) {

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:

			var err error
			msg, err := a.stream.Recv()
			if err != nil {
				a.log.Errorf("connection closed with err: %v", err)
				time.AfterFunc(a.cfg.ReconnectDelay, func() {
					a.Restart(ctx)
				})
				return
			}
			a.log.Infof("received response for %s: %v", msg.TypeUrl, msg.Resources)
			if handler, found := a.cfg.Handlers[msg.TypeUrl]; found {
				if err := handler.Handle(a.cfg.RemoteName, msg.Resources); err != nil {
					a.log.Infof("error handling resource %s: %v", msg.TypeUrl, err)
				}
			} else {
				a.log.Infof("no handler found for type: %s", msg.TypeUrl)
			}
		}
	}
}
