package istio

import (
	"context"
	"fmt"
	"sort"

	"github.com/jewertow/federation/internal/pkg/config"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/istio/pkg/kube"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1 "k8s.io/client-go/listers/core/v1"
)

type GatewayUpdater struct {
	cfg           config.Federation
	client        kube.Client
	serviceLister v1.ServiceLister
}

func NewGatewayUpdater(cfg config.Federation, client kube.Client, serviceLister v1.ServiceLister) *GatewayUpdater {
	return &GatewayUpdater{
		cfg:           cfg,
		client:        client,
		serviceLister: serviceLister,
	}
}

func (g *GatewayUpdater) Update() error {
	var hosts []string
	for _, exportLabelSelector := range g.cfg.ExportedServiceSet.GetLabelSelectors() {
		matchLabels := labels.SelectorFromSet(exportLabelSelector.MatchLabels)
		services, err := g.serviceLister.List(matchLabels)
		if err != nil {
			return fmt.Errorf("error listing services (selector=%s): %v", matchLabels, err)
		}
		for _, svc := range services {
			hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace))
		}
	}
	if len(hosts) == 0 {
		return nil
	}
	// ServiceLister.List is not idempotent, so to avoid redundant XDS push from Istio to proxies,
	// we must return hostnames in the same order.
	sort.Strings(hosts)

	gateway := v1alpha3.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "federation-ingress-gateway",
			Namespace: g.cfg.MeshPeers.Local.ControlPlane.Namespace,
		},
		Spec: istionetv1alpha3.Gateway{
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
		},
	}

	_, err := g.client.Istio().NetworkingV1alpha3().Gateways(gateway.Namespace).Update(context.Background(), &gateway, metav1.UpdateOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err := g.client.Istio().NetworkingV1alpha3().Gateways(gateway.Namespace).Create(context.Background(), &gateway, metav1.CreateOptions{})
			return err
		}
		return fmt.Errorf("error updating gateway: %v", err)
	}
	return nil
}
