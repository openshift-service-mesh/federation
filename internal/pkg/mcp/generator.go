package mcp

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	mcpv1alpha1 "istio.io/api/mcp/v1alpha1"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
)

type ResourceGenerator struct {
	exportedServiceCache *ServiceCache
}

func NewResourceGenerator(exportedServiceCache *ServiceCache) *ResourceGenerator {
	return &ResourceGenerator{exportedServiceCache}
}

func (g *ResourceGenerator) generateGatewayForExportedServices() ([]*anypb.Any, error) {
	var hosts []string
	for _, svcInfo := range g.exportedServiceCache.List() {
		hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local", svcInfo.Name, svcInfo.Namespace))
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
		return nil, fmt.Errorf("failed to serialize Gateway to protobuf message: %w", err)
	}
	mcpResTyped := &mcpv1alpha1.Resource{
		Metadata: &mcpv1alpha1.Metadata{
			Name: fmt.Sprintf("istio-system/mcp-federation-ingress-gateway"),
		},
		Body: mcpResBody,
	}
	mcpRes := &anypb.Any{}
	if err := anypb.MarshalFrom(mcpRes, mcpResTyped, proto.MarshalOptions{}); err != nil {
		return nil, fmt.Errorf("failed to serialize MCP resource to protobuf message: %w", err)
	}
	return []*anypb.Any{mcpRes}, nil
}
