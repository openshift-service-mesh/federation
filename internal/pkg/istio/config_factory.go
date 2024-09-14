package istio

import (
	"fmt"
	"sort"

	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1 "k8s.io/client-go/listers/core/v1"

	"github.com/jewertow/federation/internal/api/federation/v1alpha1"
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

func (cf *ConfigFactory) GenerateServiceAndWorkloadEntries(importedServices []*v1alpha1.ExportedService) ([]*v1alpha3.ServiceEntry, []*v1alpha3.WorkloadEntry, error) {
	var serviceEntries []*v1alpha3.ServiceEntry
	var workloadEntries []*v1alpha3.WorkloadEntry
	for _, importedSvc := range importedServices {
		// enable Istio mTLS
		importedSvc.Labels["security.istio.io/tlsMode"] = "istio"

		_, err := cf.serviceLister.Services(importedSvc.Namespace).Get(importedSvc.Name)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, nil, fmt.Errorf("failed to get Service %s/%s: %v", importedSvc.Name, importedSvc.Namespace, err)
			}
			// Service doesn't exist - create ServiceEntry.
			var ports []*istionetv1alpha3.ServicePort
			for _, port := range importedSvc.Ports {
				ports = append(ports, &istionetv1alpha3.ServicePort{
					Name:       port.Name,
					Number:     port.Number,
					Protocol:   port.Protocol,
					TargetPort: port.TargetPort,
				})
			}
			serviceEntries = append(serviceEntries, &v1alpha3.ServiceEntry{
				ObjectMeta: metav1.ObjectMeta{
					// TODO: add peer name to ensure uniqueness when more than 2 peers are connected
					Name:      fmt.Sprintf("import_%s_%s", importedSvc.Name, importedSvc.Namespace),
					Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
				},
				Spec: istionetv1alpha3.ServiceEntry{
					Hosts:      []string{fmt.Sprintf("%s.%s.svc.cluster.local", importedSvc.Name, importedSvc.Namespace)},
					Ports:      ports,
					Endpoints:  cf.makeWorkloadEntrySpecs(importedSvc.Ports, importedSvc.Labels),
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istionetv1alpha3.ServiceEntry_STATIC,
				},
			})
		} else {
			// Service already exists - create WorkloadEntries.
			workloadEntrySpecs := cf.makeWorkloadEntrySpecs(importedSvc.Ports, importedSvc.Labels)
			for idx, weSpec := range workloadEntrySpecs {
				workloadEntries = append(workloadEntries, &v1alpha3.WorkloadEntry{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("import_%s_%d", importedSvc.Name, idx),
						Namespace: importedSvc.Namespace,
					},
					Spec: *weSpec.DeepCopy(),
				})
			}
		}
	}
	return serviceEntries, workloadEntries, nil
}

func (cf *ConfigFactory) makeWorkloadEntrySpecs(ports []*v1alpha1.ServicePort, labels map[string]string) []*istionetv1alpha3.WorkloadEntry {
	var workloadEntries []*istionetv1alpha3.WorkloadEntry
	for _, addr := range cf.cfg.MeshPeers.Remote.DataPlane.Addresses {
		we := &istionetv1alpha3.WorkloadEntry{
			Address: addr,
			Network: cf.cfg.MeshPeers.Remote.Network,
			Labels:  labels,
			Ports:   make(map[string]uint32, len(ports)),
		}
		for _, p := range ports {
			we.Ports[p.Name] = cf.cfg.GetRemoteDataPlaneGatewayPort()
		}
		workloadEntries = append(workloadEntries, we)
	}
	return workloadEntries
}
