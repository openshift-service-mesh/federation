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
	reconnectDelay                     = 5 * time.Second
)

var log = istiolog.RegisterScope("adsc", "Aggregated Discovery Service Client")

type ADSCConfig struct {
	DiscoveryAddr            string
	InitialDiscoveryRequests []*discovery.DiscoveryRequest
	Handlers                 map[string]ResponseHandler
}

type ADSC struct {
	stream discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient
	// xds client used to create a stream
	client discovery.AggregatedDiscoveryServiceClient
	conn   *grpc.ClientConn
	cfg    *ADSCConfig
}

func New(opts *ADSCConfig) (*ADSC, error) {
	if opts == nil {
		return nil, errors.New("adsc: opts is nil")
	}
	adsc := &ADSC{cfg: opts}
	if err := adsc.dial(); err != nil {
		return nil, err
	}

	return adsc, nil
}

func (a *ADSC) Run() error {
	var err error
	a.client = discovery.NewAggregatedDiscoveryServiceClient(a.conn)
	a.stream, err = a.client.StreamAggregatedResources(context.Background())
	if err != nil {
		return err
	}
	// Send the initial requests
	for _, r := range a.cfg.InitialDiscoveryRequests {
		_ = a.send(r)
	}

	go a.handleRecv()
	return nil
}

func (a *ADSC) Restart() {
	log.Infof("reconnecting to ADS server %s", a.cfg.DiscoveryAddr)
	err := a.Run()
	if err != nil {
		log.Errorf("failed to Restart to ADS server %s: %v", a.cfg.DiscoveryAddr, err)
		time.AfterFunc(reconnectDelay, a.Restart)
	}
}

func (a *ADSC) send(req *discovery.DiscoveryRequest) error {
	req.ResponseNonce = time.Now().String()
	log.Infof("Sending Discovery Request to ADS server: %s", req.String())
	return a.stream.Send(req)
}

func (a *ADSC) dial() error {
	backoffConfig := backoff.DefaultConfig
	backoffConfig.MaxDelay = reconnectDelay

	var err error
	a.conn, err = grpc.NewClient(
		a.cfg.DiscoveryAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithInitialWindowSize(int32(defaultInitialWindowSize)),
		grpc.WithInitialConnWindowSize(int32(defaultInitialConnWindowSize)),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(defaultClientMaxReceiveMessageSize)),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: reconnectDelay,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to establish connection to the ADS server %s: %w", a.cfg.DiscoveryAddr, err)
	}
	return nil
}

func (a *ADSC) handleRecv() {
	for {
		var err error
		msg, err := a.stream.Recv()
		if err != nil {
			log.Errorf("connection closed with err: %v", err)
			time.AfterFunc(reconnectDelay, a.Restart)
			return
		}
		log.Infof("received response for %s: %v", msg.TypeUrl, msg.Resources)
		if handler, found := a.cfg.Handlers[msg.TypeUrl]; found {
			if err := handler.Handle(msg.Resources); err != nil {
				log.Infof("error handling resource %s: %v", msg.TypeUrl, err)
			}
		} else {
			log.Infof("no handler found for type: %s", msg.TypeUrl)
		}
	}
}
