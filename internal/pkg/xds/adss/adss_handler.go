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
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	envoycfgcorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	istiolog "istio.io/istio/pkg/log"

	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

var log = istiolog.RegisterScope("adss", "Aggregated Discovery Service Server")

// DeltaDiscoveryStream is a server interface for XDS.
// DeltaDiscoveryStream is a server interface for Delta XDS.
type (
	DiscoveryStream      = discovery.AggregatedDiscoveryService_StreamAggregatedResourcesServer
	DeltaDiscoveryStream = discovery.AggregatedDiscoveryService_DeltaAggregatedResourcesServer
)

// adsServer implements Envoy's AggregatedDiscoveryService.
type adsServer struct {
	handlers         map[string]RequestHandler
	subscribers      sync.Map
	nextSubscriberID atomic.Uint64
}

// subscriber represents a client that is subscribed to XDS resources.
type subscriber struct {
	id          uint64
	stream      DiscoveryStream
	closeStream func()
}

var _ discovery.AggregatedDiscoveryServiceServer = (*adsServer)(nil)

func (adss *adsServer) StreamAggregatedResources(downstream DiscoveryStream) error {
	log.Info("New subscriber connected")
	ctx, closeStream := context.WithCancel(downstream.Context())

	sub := &subscriber{
		id:          adss.nextSubscriberID.Add(1),
		stream:      downstream,
		closeStream: closeStream,
	}

	adss.subscribers.Store(sub.id, sub)

	go adss.recvFromStream(int64(sub.id), downstream)

	<-ctx.Done()
	return nil
}

// DeltaAggregatedResources is not implemented.
func (adss *adsServer) DeltaAggregatedResources(downstream DeltaDiscoveryStream) error {
	return status.Errorf(codes.Unimplemented, "Not Implemented")
}

var (
	maxUintDigits = len(strconv.FormatUint(uint64(math.MaxUint64), 10))
	subIDFmtStr   = `%0` + strconv.Itoa(maxUintDigits) + `d`
)

// recvFromStream receives discovery requests from the subscriber.
func (adss *adsServer) recvFromStream(id int64, downstream DiscoveryStream) {
	log.Infof("Received from stream %d", id)
	for {
		discoveryRequest, err := downstream.Recv()
		if err != nil {
			log.Errorf("error while recv discovery request from subscriber %s: %v", fmt.Sprintf(subIDFmtStr, id), err)
			break
		}
		log.Infof("Got discovery request from subscriber %s: %v", fmt.Sprintf(subIDFmtStr, id), discoveryRequest)
		if discoveryRequest.GetVersionInfo() == "" {
			resources, err := adss.generateResources(discoveryRequest.GetTypeUrl())
			if err != nil {
				// TODO: Do not push empty resources if there was an error during resource generation,
				// because that may cause unintentional removal of the subscribed resources.
				log.Errorf("failed to generate resources of type %s: %v", discoveryRequest.GetTypeUrl(), err)
			}
			log.Infof("Sending initial config snapshot for type %s: %s", discoveryRequest.GetTypeUrl(), resources)
			if err := sendToStream(downstream, discoveryRequest.GetTypeUrl(), resources, strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
				log.Errorf("failed to send initial config snapshot for type %s: %v", discoveryRequest.GetTypeUrl(), err)
			}
		}
	}
}

func (adss *adsServer) generateResources(typeUrl string) ([]*anypb.Any, error) {
	handler, found := adss.handlers[typeUrl]
	if !found {
		return []*anypb.Any{}, nil
	}

	log.Infof("Generating config snapshot for type %s", typeUrl)
	resources, err := handler.GenerateResponse()
	if err != nil {
		log.Errorf("failed generating resources for type %s: %v", typeUrl, err)
		return []*anypb.Any{}, fmt.Errorf("failed generating resources for type %s: %w", typeUrl, err)
	}
	return resources, nil
}

// sendToStream sends XDS resources to the subscriber.
func sendToStream(downstream DiscoveryStream, typeUrl string, xdsResources []*anypb.Any, version string) error {
	if err := downstream.Send(&discovery.DiscoveryResponse{
		TypeUrl:     typeUrl,
		VersionInfo: version,
		Resources:   xdsResources,
		ControlPlane: &envoycfgcorev3.ControlPlane{
			Identifier: os.Getenv("POD_NAME"),
		},
		Nonce: version,
	}); err != nil {
		return err
	}
	return nil
}

func (adss *adsServer) subscribersLen() int {
	length := 0
	adss.subscribers.Range(func(_, _ interface{}) bool {
		length++
		return true
	})
	return length
}

func (adss *adsServer) push(pushRequest xds.PushRequest) error {
	if adss.subscribersLen() == 0 {
		log.Infof("Skip pushing XDS resources for request [type=%s,resources=%v] as there are no subscribers", pushRequest.TypeUrl, pushRequest.Resources)
		return nil
	}

	resources := pushRequest.Resources
	if resources == nil {
		var err error
		resources, err = adss.generateResources(pushRequest.TypeUrl)
		if err != nil {
			return err
		}
	}

	log.Infof("Pushing discovery response to subscribers: [type=%s,resources=%v]", pushRequest.TypeUrl, resources)
	adss.subscribers.Range(func(key, value any) bool {
		log.Infof("Sending to subscriber %s", fmt.Sprintf(subIDFmtStr, key.(uint64)))
		if err := value.(*subscriber).stream.Send(&discovery.DiscoveryResponse{
			TypeUrl:     pushRequest.TypeUrl,
			VersionInfo: strconv.FormatInt(time.Now().Unix(), 10), // TODO improve version computation
			Resources:   resources,
			ControlPlane: &envoycfgcorev3.ControlPlane{
				Identifier: os.Getenv("POD_NAME"),
			},
		}); err != nil {
			log.Errorf("error sending XDS resources: %v", err)
			value.(*subscriber).closeStream()
			adss.subscribers.Delete(key)
		}
		return true
	})
	return nil
}

// closeSubscribers closes all active subscriber streams.
func (adss *adsServer) closeSubscribers() {
	adss.subscribers.Range(func(key, value any) bool {
		log.Infof("Closing stream of subscriber %s", fmt.Sprintf(subIDFmtStr, key.(uint64)))
		value.(*subscriber).closeStream()
		adss.subscribers.Delete(key)
		return true
	})
}
