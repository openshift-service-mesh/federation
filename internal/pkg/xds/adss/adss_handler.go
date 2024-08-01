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
	"github.com/jewertow/federation/internal/pkg/xds"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/klog/v2"
)

// DeltaDiscoveryStream is a server interface for XDS.
// DeltaDiscoveryStream is a server interface for Delta XDS.
type (
	DiscoveryStream      = discovery.AggregatedDiscoveryService_StreamAggregatedResourcesServer
	DeltaDiscoveryStream = discovery.AggregatedDiscoveryService_DeltaAggregatedResourcesServer
)

// adsServer implements Envoy's AggregatedDiscoveryService service for sending MCP resources to Istiod.
// ads is Aggregated Discovery Service
type adsServer struct {
	handlers         map[string]RequestHandler
	subscribers      sync.Map
	nextSubscriberID atomic.Uint64
	serverID         string
}

// subscriber represents a client that is subscribed to MCP resources.
type subscriber struct {
	id          uint64
	stream      DiscoveryStream
	closeStream func()
}

var _ discovery.AggregatedDiscoveryServiceServer = (*adsServer)(nil)

// NewADSServer creates a new instance of the AggregatedDiscoveryServiceServer.
func (adss *adsServer) StreamAggregatedResources(downstream DiscoveryStream) error {
	klog.Infof("[%s] New subscriber connected", adss.serverID)
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
	klog.Infof("[%s] Received from stream %d", adss.serverID, id)
	for {
		discoveryRequest, err := downstream.Recv()
		if err != nil {
			klog.Errorf("[%s] error while recv discovery request from subscriber %s: %v", adss.serverID, fmt.Sprintf(subIDFmtStr, id), err)
			break
		}
		klog.Infof("[%s] Got discovery request from subscriber %s: %v", adss.serverID, fmt.Sprintf(subIDFmtStr, id), discoveryRequest)
		if discoveryRequest.GetVersionInfo() == "" {
			resources, err := adss.generateResources(discoveryRequest.GetTypeUrl())
			if len(resources) == 0 || err != nil {
				klog.Infof("[%s] Sending initial empty config snapshot for type %s", adss.serverID, discoveryRequest.GetTypeUrl())
			}
			if err := sendToStream(downstream, discoveryRequest.GetTypeUrl(), resources, strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
				klog.Errorf("[%s] failed to send initial config snapshot for type %s: %v", adss.serverID, discoveryRequest.GetTypeUrl(), err)
			}
		}
	}
}

func (adss *adsServer) generateResources(typeUrl string) ([]*anypb.Any, error) {
	if handler, found := adss.handlers[typeUrl]; found {
		klog.Infof("[%s] Generating config snapshot for type %s", adss.serverID, typeUrl)
		if resources, err := handler.GenerateResponse(); err != nil {
			klog.Errorf("[%s] error generating resources of type %s: %v", adss.serverID, typeUrl, err)
			return []*anypb.Any{}, err
		} else {
			return resources, nil
		}
	}
	return []*anypb.Any{}, nil
}

// sendToStream sends MCP resources to the subscriber.
func sendToStream(downstream DiscoveryStream, typeUrl string, mcpResources []*anypb.Any, version string) error {
	if err := downstream.Send(&discovery.DiscoveryResponse{
		TypeUrl:     typeUrl,
		VersionInfo: version,
		Resources:   mcpResources,
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
		klog.Infof("[%s] Skip pushing XDS resources for request [type=%s,resources=%v] as there are no subscribers", adss.serverID, pushRequest.TypeUrl, pushRequest.Resources)
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

	klog.Infof("[%s] Pushing discovery response to subscribers: [type=%s,resources=%v]", adss.serverID, pushRequest.TypeUrl, resources)
	adss.subscribers.Range(func(key, value any) bool {
		klog.Infof("[%s] Sending to subscriber %s", adss.serverID, fmt.Sprintf(subIDFmtStr, key.(uint64)))
		if err := value.(*subscriber).stream.Send(&discovery.DiscoveryResponse{
			TypeUrl:     pushRequest.TypeUrl,
			VersionInfo: strconv.FormatInt(time.Now().Unix(), 10), // TODO improve version computation
			Resources:   resources,
			ControlPlane: &envoycfgcorev3.ControlPlane{
				Identifier: os.Getenv("POD_NAME"),
			},
		}); err != nil {
			klog.Errorf("[%s] error sending MCP resources: %v", adss.serverID, err)
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
		klog.Infof("[%s] Closing stream of subscriber %s", adss.serverID, fmt.Sprintf(subIDFmtStr, key.(uint64)))
		value.(*subscriber).closeStream()
		adss.subscribers.Delete(key)

		return true
	})
}
