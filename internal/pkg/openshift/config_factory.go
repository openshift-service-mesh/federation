package openshift

import (
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	v1 "k8s.io/client-go/listers/core/v1"

	"github.com/openshift-service-mesh/federation/internal/pkg/config"
)

type ConfigFactory struct {
	cfg           config.Federation
	serviceLister v1.ServiceLister
}

func NewConfigFactory(
	cfg config.Federation,
	serviceLister v1.ServiceLister,
) *ConfigFactory {
	return &ConfigFactory{
		cfg:           cfg,
		serviceLister: serviceLister,
	}
}

func (cf *ConfigFactory) Routes() ([]*routev1.Route, error) {
	createRoute := func(svcName, svcNamespace string, port int32) *routev1.Route {
		return &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s-%d-to-federation-ingress-gateway", svcName, svcNamespace, port),
				Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
				Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
			},
			Spec: routev1.RouteSpec{
				Host: fmt.Sprintf("%s-%d.%s.svc.cluster.local", svcName, port, svcNamespace),
				To: routev1.RouteTargetReference{
					Kind: "Service",
					Name: "federation-ingress-gateway",
				},
				Port: &routev1.RoutePort{
					TargetPort: intstr.FromString("tls-passthrough"),
				},
				TLS: &routev1.TLSConfig{
					Termination: routev1.TLSTerminationPassthrough,
				},
			},
		}
	}

	routes := []*routev1.Route{
		createRoute("federation-discovery-service", "istio-system", 15080),
	}
	for _, exportLabelSelector := range cf.cfg.ExportedServiceSet.GetLabelSelectors() {
		matchLabels := labels.SelectorFromSet(exportLabelSelector.MatchLabels)
		services, err := cf.serviceLister.List(matchLabels)
		if err != nil {
			return nil, fmt.Errorf("error listing services (selector=%s): %w", matchLabels, err)
		}
		for _, svc := range services {
			for _, port := range svc.Spec.Ports {
				routes = append(routes, createRoute(svc.Name, svc.Namespace, port.Port))
			}
		}
	}
	return routes, nil
}
