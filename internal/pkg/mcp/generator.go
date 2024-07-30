package mcp

import (
	"fmt"

	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	mcpv1alpha1 "istio.io/api/mcp/v1alpha1"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

var _ xds.ResourceGenerator = (*gatewayResourceGenerator)(nil)

type gatewayResourceGenerator struct {
	typeUrl         string
	cfg             config.Federation
	serviceInformer cache.SharedIndexInformer
}

func NewGatewayResourceGenerator(cfg config.Federation, informerFactory informers.SharedInformerFactory) *gatewayResourceGenerator {
	return &gatewayResourceGenerator{
		"networking.istio.io/v1alpha3/Gateway",
		cfg,
		informerFactory.Core().V1().Services().Informer(),
	}
}

func (g *gatewayResourceGenerator) GetTypeUrl() string {
	return g.typeUrl
}

func (g *gatewayResourceGenerator) Generate() ([]*anypb.Any, error) {
	var hosts []string
	for _, obj := range g.serviceInformer.GetStore().List() {
		svc := obj.(*corev1.Service)
		for _, rule := range g.cfg.ExportedServiceSet.Rules {
			for _, selectors := range rule.LabelSelectors {
				if matchesLabelSelector(svc, selectors.MatchLabels) {
					hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace))
				}
			}
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no hosts found for gateway")
	}
	gwSpec := &istionetv1alpha3.Gateway{
		Selector: map[string]string{
			"istio": "eastwestgateway",
		},
		Servers: []*istionetv1alpha3.Server{
			{
				Hosts: hosts,
				Port: &istionetv1alpha3.Port{
					Number:   15443,
					Name:     "tls",
					Protocol: "TLS",
				},
				Tls: &istionetv1alpha3.ServerTLSSettings{
					Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
				},
			},
		},
	}

	mcpResBody := &anypb.Any{}
	if err := anypb.MarshalFrom(mcpResBody, gwSpec, proto.MarshalOptions{}); err != nil {
		return []*anypb.Any{}, fmt.Errorf("failed to serialize Gateway to protobuf message: %w", err)
	}
	mcpResTyped := &mcpv1alpha1.Resource{
		Metadata: &mcpv1alpha1.Metadata{
			Name: "istio-system/mcp-federation-ingress-gateway",
		},
		Body: mcpResBody,
	}
	mcpRes := &anypb.Any{}
	if err := anypb.MarshalFrom(mcpRes, mcpResTyped, proto.MarshalOptions{}); err != nil {
		return []*anypb.Any{}, fmt.Errorf("failed to serialize MCP resource to protobuf message: %w", err)
	}
	return []*anypb.Any{mcpRes}, nil
}
