package mcp

import (
	"fmt"

	"github.com/jewertow/federation/internal/pkg/config"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	mcpv1alpha1 "istio.io/api/mcp/v1alpha1"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

type resourceGenerator struct {
	cfg             config.Federation
	serviceInformer cache.SharedIndexInformer
}

func newResourceGenerator(cfg config.Federation, serviceInformer cache.SharedIndexInformer) *resourceGenerator {
	return &resourceGenerator{cfg, serviceInformer}
}

func (g *resourceGenerator) generateGatewayForExportedServices() (McpResources, error) {
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
	gwSpec := &istionetv1alpha3.Gateway{
		Selector: map[string]string{
			"istio": "eastwestgateway",
		},
		Servers: []*istionetv1alpha3.Server{
			{
				Port: &istionetv1alpha3.Port{
					Number:   15443,
					Name:     "tls",
					Protocol: "TLS",
				},
				Hosts: hosts,
				Tls: &istionetv1alpha3.ServerTLSSettings{
					Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
				},
			},
		},
	}

	mcpResBody := &anypb.Any{}
	if err := anypb.MarshalFrom(mcpResBody, gwSpec, proto.MarshalOptions{}); err != nil {
		return McpResources{}, fmt.Errorf("failed to serialize Gateway to protobuf message: %w", err)
	}
	mcpResTyped := &mcpv1alpha1.Resource{
		Metadata: &mcpv1alpha1.Metadata{
			Name: fmt.Sprintf("istio-system/mcp-federation-ingress-gateway"),
		},
		Body: mcpResBody,
	}
	mcpRes := &anypb.Any{}
	if err := anypb.MarshalFrom(mcpRes, mcpResTyped, proto.MarshalOptions{}); err != nil {
		return McpResources{}, fmt.Errorf("failed to serialize MCP resource to protobuf message: %w", err)
	}
	return McpResources{
		TypeUrl:   "networking.istio.io/v1alpha3/Gateway",
		Resources: []*anypb.Any{mcpRes},
	}, nil
}
