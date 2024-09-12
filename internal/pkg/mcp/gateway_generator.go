package mcp

import (
	"fmt"
	"sort"

	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"google.golang.org/protobuf/types/known/anypb"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	istiocfg "istio.io/istio/pkg/config"
	"k8s.io/apimachinery/pkg/labels"
	v1 "k8s.io/client-go/listers/core/v1"
)

var _ adss.RequestHandler = (*GatewayResourceGenerator)(nil)

// GatewayResourceGenerator generates Istio Gateway for all Services matching export rules.
type GatewayResourceGenerator struct {
	cfg           config.Federation
	serviceLister v1.ServiceLister
}

func NewGatewayResourceGenerator(cfg config.Federation, serviceLister v1.ServiceLister) *GatewayResourceGenerator {
	return &GatewayResourceGenerator{
		cfg:           cfg,
		serviceLister: serviceLister,
	}
}

func (g *GatewayResourceGenerator) GetTypeUrl() string {
	return xds.GatewayTypeUrl
}

func (g *GatewayResourceGenerator) GenerateResponse() ([]*anypb.Any, error) {
	var hosts []string
	for _, exportLabelSelector := range g.cfg.ExportedServiceSet.GetLabelSelectors() {
		matchLabels := labels.SelectorFromSet(exportLabelSelector.MatchLabels)
		services, err := g.serviceLister.List(matchLabels)
		if err != nil {
			return nil, fmt.Errorf("error listing services (selector=%s): %v", matchLabels, err)
		}
		for _, svc := range services {
			hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace))
		}
	}
	if len(hosts) == 0 {
		return nil, nil
	}
	// ServiceLister.List is not idempotent, so to avoid redundant XDS push from Istio to proxies,
	// we must return hostnames in the same order.
	sort.Strings(hosts)

	gwSpec := &istionetv1alpha3.Gateway{
		Selector: g.cfg.MeshPeers.Local.Gateways.DataPlane.Selector,
		Servers: []*istionetv1alpha3.Server{{
			Hosts: hosts,
			Port: &istionetv1alpha3.Port{
				Number:   g.cfg.GetLocalDataPlaneGatewayPort(),
				Name:     "tls",
				Protocol: "TLS",
			},
			Tls: &istionetv1alpha3.ServerTLSSettings{
				Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
			},
		}},
	}

	return serialize(&istiocfg.Config{
		Meta: istiocfg.Meta{
			Name:      "mcp-federation-ingress-gateway",
			Namespace: g.cfg.GetLocalDataPlaneGatewayNamespace(),
		},
		Spec: gwSpec,
	})
}
