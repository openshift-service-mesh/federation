package mcp

import (
	"fmt"

	"github.com/jewertow/federation/internal/pkg/common"
	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	"google.golang.org/protobuf/types/known/anypb"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	istiocfg "istio.io/istio/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

var _ adss.RequestHandler = (*gatewayResourceGenerator)(nil)

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

func (g *gatewayResourceGenerator) GenerateResponse() ([]*anypb.Any, error) {
	var hosts []string
	for _, obj := range g.serviceInformer.GetStore().List() {
		svc := obj.(*corev1.Service)
		if common.MatchExportRules(svc, g.cfg.ExportedServiceSet.GetLabelSelectors()) {
			// TODO: should we also append "${name}.${ns}" and "${name}.${ns}.svc"?
			hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace))
		}
	}
	if len(hosts) == 0 {
		return nil, nil
	}

	gwSpec := &istionetv1alpha3.Gateway{
		Selector: map[string]string{
			"istio": "eastwestgateway",
		},
		Servers: []*istionetv1alpha3.Server{{
			Hosts: hosts,
			Port: &istionetv1alpha3.Port{
				Number:   15443,
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
			Name: "mcp-federation-ingress-gateway",
			// TODO: set from config
			Namespace: "istio-system",
		},
		Spec: gwSpec,
	})
}
