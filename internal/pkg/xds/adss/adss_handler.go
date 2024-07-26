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
	generators       map[string]xds.ResourceGenerator
	subscribers      sync.Map
	nextSubscriberID atomic.Uint64
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
	klog.Infof("New subscriber connected")
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
	klog.Infof("Received from stream %d", id)
recvLoop:
	for {
		discoveryRequest, err := downstream.Recv()
		if err != nil {
			klog.Errorf("Error while recv discovery request from subscriber %s: %v", fmt.Sprintf(subIDFmtStr, id), err)
			break recvLoop
		}
		klog.Infof("Got discovery request from subscriber %s: %v", fmt.Sprintf(subIDFmtStr, id), discoveryRequest)
		if discoveryRequest.GetVersionInfo() == "" {
			resources, err := adss.generateResources(discoveryRequest.GetTypeUrl())
			if len(resources) == 0 || err != nil {
				klog.Infof("Sending initial empty config snapshot for type %s", discoveryRequest.GetTypeUrl())
			}
			if err := sendToStream(downstream, discoveryRequest.GetTypeUrl(), resources, strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
				klog.Errorf("failed to send initial config snapshot for type %s: %v", discoveryRequest.GetTypeUrl(), err)
			}
		}
	}
}

func (adss *adsServer) generateResources(typeUrl string) ([]*anypb.Any, error) {
	if generator, ok := adss.generators[typeUrl]; ok {
		klog.Infof("Generating config snapshot for type %s", typeUrl)
		if resources, err := generator.Generate(); err != nil {
			klog.Errorf("Error generating resources of type %s: %v", typeUrl, err)
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
		klog.Infof("Skip pushing MCP event: %v as there are no subscribers", pushRequest)
		return nil
	}

	klog.Infof("Pushing MCP event to subscribers: %v", pushRequest)
	resources, err := adss.generateResources(pushRequest.TypeUrl)
	if err != nil {
		return err
	}
	adss.subscribers.Range(func(key, value any) bool {
		klog.Infof("Sending to subscriber %s", fmt.Sprintf(subIDFmtStr, key.(uint64)))
		if err := value.(*subscriber).stream.Send(&discovery.DiscoveryResponse{
			TypeUrl:     pushRequest.TypeUrl,
			VersionInfo: strconv.FormatInt(time.Now().Unix(), 10), // TODO improve version computation
			Resources:   resources,
			ControlPlane: &envoycfgcorev3.ControlPlane{
				Identifier: os.Getenv("POD_NAME"),
			},
		}); err != nil {
			klog.Errorf("Error sending MCP resources: %v", err)
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
		klog.Infof("Closing stream of subscriber %s", fmt.Sprintf(subIDFmtStr, key.(uint64)))
		value.(*subscriber).closeStream()
		adss.subscribers.Delete(key)

		return true
	})
}