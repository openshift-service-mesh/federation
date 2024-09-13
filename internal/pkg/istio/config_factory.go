package istio

import (
	"fmt"
	"sort"

	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1 "k8s.io/client-go/listers/core/v1"

	"github.com/jewertow/federation/internal/pkg/config"
)

type ConfigFactory struct {
	cfg           config.Federation
	serviceLister v1.ServiceLister
}

func NewConfigFactory(cfg config.Federation, serviceLister v1.ServiceLister) *ConfigFactory {
	return &ConfigFactory{
		cfg:           cfg,
		serviceLister: serviceLister,
	}
}

func (cf *ConfigFactory) GenerateIngressGateway() (*v1alpha3.Gateway, error) {
	var hosts []string
	for _, exportLabelSelector := range cf.cfg.ExportedServiceSet.GetLabelSelectors() {
		matchLabels := labels.SelectorFromSet(exportLabelSelector.MatchLabels)
		services, err := cf.serviceLister.List(matchLabels)
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

	gateway := &v1alpha3.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "federation-ingress-gateway",
			Namespace: cf.cfg.GetLocalIngressGatewayNamespace(),
		},
		Spec: istionetv1alpha3.Gateway{
			Selector: cf.cfg.MeshPeers.Local.Gateways.Ingress.Selector,
			Servers: []*istionetv1alpha3.Server{{
				Hosts: hosts,
				Port: &istionetv1alpha3.Port{
					Number:   cf.cfg.GetLocalIngressGatewayPort(),
					Name:     "tls",
					Protocol: "TLS",
				},
				Tls: &istionetv1alpha3.ServerTLSSettings{
					Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
				},
			}},
		},
	}

	return gateway, nil
}
