// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package istio

import (
	"fmt"
	"google.golang.org/protobuf/types/known/structpb"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/protomarshal"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1 "k8s.io/client-go/listers/core/v1"
	"net"
	"sort"

	"github.com/openshift-service-mesh/federation/internal/api/federation/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/fds"
)

const (
	federationIngressGatewayName = "federation-ingress-gateway"
)

type ConfigFactory struct {
	cfg                                 config.Federation
	serviceLister                       v1.ServiceLister
	importedServiceStore                *fds.ImportedServiceStore
	localFederationDiscoveryServiceFQDN string
}

func NewConfigFactory(
	cfg config.Federation,
	serviceLister v1.ServiceLister,
	importedServiceStore *fds.ImportedServiceStore,
	localFederationDiscoveryServiceFQDN string,
) *ConfigFactory {
	return &ConfigFactory{
		cfg:                                 cfg,
		serviceLister:                       serviceLister,
		importedServiceStore:                importedServiceStore,
		localFederationDiscoveryServiceFQDN: localFederationDiscoveryServiceFQDN,
	}
}

// DestinationRules returns destination rules to customize SNI in the mTLS connection to remote services.
// TODO: namespace in the SNI should come from remote.identity.namespace
func (cf *ConfigFactory) DestinationRules() []*v1alpha3.DestinationRule {
	if len(cf.cfg.MeshPeers.Remote.Addresses) == 0 {
		return nil
	}
	var destinationRules []*v1alpha3.DestinationRule
	if cf.cfg.MeshPeers.Remote.IngressType == config.OpenShiftRouter {
		destinationRules = append(destinationRules, &v1alpha3.DestinationRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "originate-istio-mtls-to-remote-federation-controller",
				Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
				Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
			},
			Spec: istionetv1alpha3.DestinationRule{
				Host: cf.cfg.MeshPeers.Remote.Addresses[0],
				TrafficPolicy: &istionetv1alpha3.TrafficPolicy{
					Tls: &istionetv1alpha3.ClientTLSSettings{
						Mode: istionetv1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
						Sni:  "federation-discovery-service-15080.istio-system.svc.cluster.local",
					},
				},
			},
		})
	} else {
		destinationRules = append(destinationRules, &v1alpha3.DestinationRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "originate-istio-mtls-to-remote-federation-controller",
				Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
				Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
			},
			Spec: istionetv1alpha3.DestinationRule{
				Host: fmt.Sprintf("remote-federation-controller.%s.svc.cluster.local", cf.cfg.MeshPeers.Local.ControlPlane.Namespace),
				TrafficPolicy: &istionetv1alpha3.TrafficPolicy{
					Tls: &istionetv1alpha3.ClientTLSSettings{
						Mode: istionetv1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
						Sni:  "federation-discovery-service.istio-system.svc.cluster.local",
					},
				},
			},
		})
	}
	for _, svc := range cf.importedServiceStore.GetAll() {
		dr := &v1alpha3.DestinationRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("originate-istio-mtls-to-%s-%s", svc.Name, svc.Namespace),
				Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
				Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
			},
			Spec: istionetv1alpha3.DestinationRule{
				Host: fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace),
				TrafficPolicy: &istionetv1alpha3.TrafficPolicy{
					PortLevelSettings: []*istionetv1alpha3.TrafficPolicy_PortTrafficPolicy{},
				},
			},
		}
		for _, port := range svc.Ports {
			dr.Spec.TrafficPolicy.PortLevelSettings = append(dr.Spec.TrafficPolicy.PortLevelSettings, &istionetv1alpha3.TrafficPolicy_PortTrafficPolicy{
				Port: &istionetv1alpha3.PortSelector{Number: port.Number},
				Tls: &istionetv1alpha3.ClientTLSSettings{
					Mode: istionetv1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
					Sni:  fmt.Sprintf("%s-%d.%s.svc.cluster.local", svc.Name, port.Number, svc.Namespace),
				},
			})
		}
		destinationRules = append(destinationRules, dr)
	}
	return destinationRules
}

func (cf *ConfigFactory) IngressGateway() (*v1alpha3.Gateway, error) {
	gateway := &v1alpha3.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      federationIngressGatewayName,
			Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
		},
		Spec: istionetv1alpha3.Gateway{
			Selector: cf.cfg.MeshPeers.Local.Gateways.Ingress.Selector,
			Servers: []*istionetv1alpha3.Server{{
				Hosts: []string{},
				Port: &istionetv1alpha3.Port{
					Number:   cf.cfg.MeshPeers.Local.Gateways.Ingress.Ports.GetDataPlanePort(),
					Name:     "tls-passthrough",
					Protocol: "TLS",
				},
				Tls: &istionetv1alpha3.ServerTLSSettings{
					Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
				},
			}},
		},
	}

	hosts := []string{cf.localFederationDiscoveryServiceFQDN}
	for _, exportLabelSelector := range cf.cfg.ExportedServiceSet.GetLabelSelectors() {
		matchLabels := labels.SelectorFromSet(exportLabelSelector.MatchLabels)
		services, err := cf.serviceLister.List(matchLabels)
		if err != nil {
			return nil, fmt.Errorf("error listing services (selector=%s): %w", matchLabels, err)
		}
		for _, svc := range services {
			hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace))
		}
	}
	// ServiceLister.List is not idempotent, so to avoid redundant XDS push from Istio to proxies,
	// we must return hostnames in the same order.
	sort.Strings(hosts)
	gateway.Spec.Servers[0].Hosts = hosts

	return gateway, nil
}

func (cf *ConfigFactory) EnvoyFilters() []*v1alpha3.EnvoyFilter {
	createEnvoyFilter := func(svcName, svcNamespace string, port int32) *v1alpha3.EnvoyFilter {
		buildPatchStruct := func(config string) *structpb.Struct {
			val := &structpb.Struct{}
			if err := protomarshal.UnmarshalString(config, val); err != nil {
				fmt.Printf("error unmarshalling envoyfilter config %q: %v", config, err)
			}
			return val
		}
		return &v1alpha3.EnvoyFilter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("sni-%s-%s-%d", svcName, svcName, port),
				Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
				Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
			},
			Spec: istionetv1alpha3.EnvoyFilter{
				WorkloadSelector: &istionetv1alpha3.WorkloadSelector{
					Labels: cf.cfg.MeshPeers.Local.Gateways.Ingress.Selector,
				},
				ConfigPatches: []*istionetv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{{
					ApplyTo: istionetv1alpha3.EnvoyFilter_FILTER_CHAIN,
					Match: &istionetv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
						ObjectTypes: &istionetv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
							Listener: &istionetv1alpha3.EnvoyFilter_ListenerMatch{
								Name: "0.0.0.0_15443",
								FilterChain: &istionetv1alpha3.EnvoyFilter_ListenerMatch_FilterChainMatch{
									Sni: fmt.Sprintf("outbound_.%d_._.%s.%s.svc.cluster.local", port, svcName, svcNamespace),
								},
							},
						},
					},
					Patch: &istionetv1alpha3.EnvoyFilter_Patch{
						Operation: istionetv1alpha3.EnvoyFilter_Patch_MERGE,
						Value:     buildPatchStruct(fmt.Sprintf(`{"filter_chain_match":{"server_names":["%s-%d.%s.svc.cluster.local"]}}`, svcName, port, svcNamespace)),
					},
				}},
			},
		}
	}

	envoyFilters := []*v1alpha3.EnvoyFilter{
		createEnvoyFilter("federation-discovery-service", "istio-system", 15080),
	}
	for _, exportLabelSelector := range cf.cfg.ExportedServiceSet.GetLabelSelectors() {
		matchLabels := labels.SelectorFromSet(exportLabelSelector.MatchLabels)
		services, err := cf.serviceLister.List(matchLabels)
		if err != nil {
			fmt.Printf("error listing services (selector=%s): %v", matchLabels, err)
		}
		for _, svc := range services {
			for _, port := range svc.Spec.Ports {
				envoyFilters = append(envoyFilters, createEnvoyFilter(svc.Name, svc.Namespace, port.Port))
			}
		}
	}
	return envoyFilters
}

func (cf *ConfigFactory) ServiceEntries() ([]*v1alpha3.ServiceEntry, error) {
	var serviceEntries []*v1alpha3.ServiceEntry
	for _, importedSvc := range cf.importedServiceStore.GetAll() {
		// enable Istio mTLS
		importedSvc.Labels["security.istio.io/tlsMode"] = "istio"

		_, err := cf.serviceLister.Services(importedSvc.Namespace).Get(importedSvc.Name)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get Service %s/%s: %w", importedSvc.Name, importedSvc.Namespace, err)
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
					Name:      fmt.Sprintf("import-%s-%s", importedSvc.Name, importedSvc.Namespace),
					Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
					Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
				},
				Spec: istionetv1alpha3.ServiceEntry{
					Hosts:      []string{fmt.Sprintf("%s.%s.svc.cluster.local", importedSvc.Name, importedSvc.Namespace)},
					Ports:      ports,
					Endpoints:  cf.makeWorkloadEntrySpecs(importedSvc.Ports, importedSvc.Labels),
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: istionetv1alpha3.ServiceEntry_STATIC,
				},
			})
		}
	}
	if se := cf.getServiceEntryForRemoteFederationController(); se != nil {
		serviceEntries = append(serviceEntries, se)
	}
	return serviceEntries, nil
}

func (cf *ConfigFactory) GetWorkloadEntries() ([]*v1alpha3.WorkloadEntry, error) {
	var workloadEntries []*v1alpha3.WorkloadEntry
	for _, importedSvc := range cf.importedServiceStore.GetAll() {
		// enable Istio mTLS
		importedSvc.Labels["security.istio.io/tlsMode"] = "istio"

		_, err := cf.serviceLister.Services(importedSvc.Namespace).Get(importedSvc.Name)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get Service %s/%s: %w", importedSvc.Name, importedSvc.Namespace, err)
			}
		} else {
			// Service already exists - create WorkloadEntries.
			workloadEntrySpecs := cf.makeWorkloadEntrySpecs(importedSvc.Ports, importedSvc.Labels)
			for idx, weSpec := range workloadEntrySpecs {
				workloadEntries = append(workloadEntries, &v1alpha3.WorkloadEntry{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("import-%s-%d", importedSvc.Name, idx),
						Namespace: importedSvc.Namespace,
						Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
					},
					Spec: *weSpec.DeepCopy(),
				})
			}
		}
	}
	return workloadEntries, nil
}

func (cf *ConfigFactory) getServiceEntryForRemoteFederationController() *v1alpha3.ServiceEntry {
	if len(cf.cfg.MeshPeers.Remote.Addresses) == 0 {
		return nil
	}

	if cf.cfg.MeshPeers.Remote.IngressType == config.OpenShiftRouter {
		// TODO: Add validation for addresses - it should be single hostname
		return &v1alpha3.ServiceEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remote-federation-controller",
				Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
				Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
			},
			Spec: istionetv1alpha3.ServiceEntry{
				Hosts:     []string{cf.cfg.MeshPeers.Remote.Addresses[0]},
				Addresses: resolveAddress(cf.cfg.MeshPeers.Remote.Addresses[0]),
				Ports: []*istionetv1alpha3.ServicePort{{
					Name:     "tls-passthrough",
					Number:   cf.cfg.MeshPeers.Remote.Ports.GetDataPlanePort(),
					Protocol: "TLS",
				}},
				Location:   istionetv1alpha3.ServiceEntry_MESH_EXTERNAL,
				Resolution: istionetv1alpha3.ServiceEntry_DNS,
			},
		}
	}

	var endpoints []*istionetv1alpha3.WorkloadEntry
	for _, remoteAddr := range cf.cfg.MeshPeers.Remote.Addresses {
		endpoints = append(endpoints, &istionetv1alpha3.WorkloadEntry{Address: remoteAddr})
	}
	return &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remote-federation-controller",
			Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
			Labels:    map[string]string{"federation.istio-ecosystem.io/peer": "todo"},
		},
		Spec: istionetv1alpha3.ServiceEntry{
			Hosts: []string{fmt.Sprintf("remote-federation-controller.%s.svc.cluster.local", cf.cfg.MeshPeers.Local.ControlPlane.Namespace)},
			Ports: []*istionetv1alpha3.ServicePort{{
				Name:     "tls-passthrough",
				Number:   cf.cfg.MeshPeers.Remote.Ports.GetDataPlanePort(),
				Protocol: "TLS",
			}},
			Location:   istionetv1alpha3.ServiceEntry_MESH_EXTERNAL,
			Resolution: istionetv1alpha3.ServiceEntry_STATIC,
			Endpoints:  endpoints,
		},
	}
}

func (cf *ConfigFactory) makeWorkloadEntrySpecs(ports []*v1alpha1.ServicePort, labels map[string]string) []*istionetv1alpha3.WorkloadEntry {
	var workloadEntries []*istionetv1alpha3.WorkloadEntry
	for _, addr := range cf.cfg.MeshPeers.Remote.Addresses {
		we := &istionetv1alpha3.WorkloadEntry{
			Address: resolveAddress(addr)[0],
			Network: cf.cfg.MeshPeers.Remote.Network,
			Labels:  labels,
			Ports:   make(map[string]uint32, len(ports)),
		}
		for _, p := range ports {
			we.Ports[p.Name] = cf.cfg.MeshPeers.Remote.Ports.GetDataPlanePort()
		}
		workloadEntries = append(workloadEntries, we)
	}
	return workloadEntries
}

// This will not work with DNS proxy, so we have to resolve this IP periodically and trigger updates on DNS change,
// or implement this in Istio if possible.
func resolveAddress(addr string) []string {
	if ip := net.ParseIP(addr); ip != nil {
		return []string{addr}
	}

	ips, err := net.LookupIP(addr)
	if err != nil {
		fmt.Printf("Failed to resolve '%s': %v\n", addr, err)
	}
	return slices.Map(ips, func(ip net.IP) string {
		return ip.String()
	})
}
