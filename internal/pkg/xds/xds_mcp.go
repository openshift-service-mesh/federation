package xds

import (
	"context"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"log"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	envoycfgcorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	mcpv1alpha1 "istio.io/api/mcp/v1alpha1"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
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
	log.Println("New subscriber connected")
	ctx, closeStream := context.WithCancel(downstream.Context())

	sub := &subscriber{
		id:          adss.nextSubscriberID.Add(1),
		stream:      downstream,
		closeStream: closeStream,
	}

	adss.subscribers.Store(sub.id, sub)
	go recvFromStream(int64(sub.id), downstream)

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
func recvFromStream(id int64, downstream DiscoveryStream) {
	log.Println("Received from stream ", id)
recvLoop:
	for {
		discoveryRequest, err := downstream.Recv()
		if err != nil {
			log.Print("Error while recv discovery request from subscriber ", fmt.Sprintf(subIDFmtStr, id), err)
			break recvLoop
		}
		log.Print("Got discovery request from subscriber ", fmt.Sprintf(subIDFmtStr, id), discoveryRequest)
		if discoveryRequest.GetVersionInfo() == "" {
			log.Print("Sending initial empty config snapshot for type ", discoveryRequest.GetTypeUrl())
			if err := sendToStream(downstream, discoveryRequest.GetTypeUrl(), make([]*anypb.Any, 0), strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
				log.Printf("failed to send initial config snapshot for type %s: %v", discoveryRequest.GetTypeUrl(), err)
			}
		}
	}
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

func (adss *adsServer) push(resources []*anypb.Any) error {
	adss.subscribers.Range(func(key, value any) bool {
		log.Print("Sending to subscriber ", fmt.Sprintf(subIDFmtStr, key.(uint64)))

		if err := value.(*subscriber).stream.Send(&discovery.DiscoveryResponse{
			TypeUrl:     "networking.istio.io/v1alpha3/Gateway",
			VersionInfo: strconv.FormatInt(time.Now().Unix(), 10),
			Resources:   resources,
			ControlPlane: &envoycfgcorev3.ControlPlane{
				Identifier: os.Getenv("POD_NAME"),
			},
		}); err != nil {
			log.Print("Error sending MCP resources: ", err)
			value.(*subscriber).closeStream()
			adss.subscribers.Delete(key)
		}
		return true
	})
	return nil
}

// pushToSubscribers pushes MCP resources to active subscribers.
func (adss *adsServer) pushToSubscribers() error {
	var mcpResources []*anypb.Any
	for _, host := range []string{"example.com", "istio.io"} {
		se, err := makeMCPServiceEntry(host)
		if err != nil {
			return fmt.Errorf("failed to create MCP service entry for the hostname %s: %v", host, err)
		}
		mcpResources = append(mcpResources, se)
	}

	adss.subscribers.Range(func(key, value any) bool {
		log.Print("Sending to subscriber ", fmt.Sprintf(subIDFmtStr, key.(uint64)))

		if err := value.(*subscriber).stream.Send(&discovery.DiscoveryResponse{
			TypeUrl:     "networking.istio.io/v1alpha3/ServiceEntry",
			VersionInfo: strconv.FormatInt(time.Now().Unix(), 10),
			Resources:   mcpResources,
			ControlPlane: &envoycfgcorev3.ControlPlane{
				Identifier: os.Getenv("POD_NAME"),
			},
		}); err != nil {
			log.Print("Error sending MCP resources: ", err)
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
		log.Print("Closing stream of subscriber ", fmt.Sprintf(subIDFmtStr, key.(uint64)))
		value.(*subscriber).closeStream()
		adss.subscribers.Delete(key)

		return true
	})
}

const numMCPResources = 100

// makeMCPServiceEntry returns an Istio ServiceEntry serialized as a protocol
// buffer message.
func makeMCPServiceEntry(hostname string) (*anypb.Any, error) {
	seSpec := &istionetv1alpha3.ServiceEntry{
		Hosts:    []string{hostname},
		Location: istionetv1alpha3.ServiceEntry_MESH_EXTERNAL,
		Ports: []*istionetv1alpha3.ServicePort{{
			Number:   443,
			Name:     "https",
			Protocol: "TLS",
		}},
		Resolution: istionetv1alpha3.ServiceEntry_DNS,
	}

	mcpResBody := &anypb.Any{}
	if err := anypb.MarshalFrom(mcpResBody, seSpec, proto.MarshalOptions{}); err != nil {
		return nil, fmt.Errorf("serializing ServiceEntry to protobuf message: %w", err)
	}

	mcpResTyped := &mcpv1alpha1.Resource{
		Metadata: &mcpv1alpha1.Metadata{
			Name: fmt.Sprintf("istio-system/mcp-se-%s", hostname),
		},
		Body: mcpResBody,
	}

	mcpRes := &anypb.Any{}
	if err := anypb.MarshalFrom(mcpRes, mcpResTyped, proto.MarshalOptions{}); err != nil {
		return nil, fmt.Errorf("serializing MCP Resource to protobuf message: %w", err)
	}

	return mcpRes, nil
}
