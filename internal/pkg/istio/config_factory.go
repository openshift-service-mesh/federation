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
	"sort"

	"google.golang.org/protobuf/types/known/structpb"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/istio/pkg/maps"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/protomarshal"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1 "k8s.io/client-go/listers/core/v1"

	"github.com/openshift-service-mesh/federation/internal/api/federation/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/fds"
	"github.com/openshift-service-mesh/federation/internal/pkg/networking"
)

const (
	federationIngressGatewayName = "federation-ingress-gateway"
)

type ConfigFactory struct {
	cfg                  config.Federation
	serviceLister        v1.ServiceLister
	importedServiceStore *fds.ImportedServiceStore
	namespace            string
}

func NewConfigFactory(
	cfg config.Federation,
	serviceLister v1.ServiceLister,
	importedServiceStore *fds.ImportedServiceStore,
	namespace string,
) *ConfigFactory {
	return &ConfigFactory{
		cfg:                  cfg,
		serviceLister:        serviceLister,
		importedServiceStore: importedServiceStore,
		namespace:            namespace,
	}
}

// DestinationRules customize SNI in the client mTLS connection when the remote ingress is openshift-router,
// because that ingress requires hosts compatible with https://datatracker.ietf.org/doc/html/rfc952.
func (cf *ConfigFactory) DestinationRules() []*v1alpha3.DestinationRule {
	remote := cf.cfg.MeshPeers.Remote

	if remote.IngressType != config.OpenShiftRouter {
		return nil
	}

	createObjectMeta := func(svcName, svcNs string) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:      fmt.Sprintf("mtls-sni-%s-%s", svcName, svcNs),
			Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
			Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
		}
	}

	destinationRules := []*v1alpha3.DestinationRule{{
		ObjectMeta: createObjectMeta(remote.ServiceName(), "istio-system"),
		Spec: istionetv1alpha3.DestinationRule{
			Host: remote.ServiceFQDN(),
			TrafficPolicy: &istionetv1alpha3.TrafficPolicy{
				Tls: &istionetv1alpha3.ClientTLSSettings{
					Mode: istionetv1alpha3.ClientTLSSettings_ISTIO_MUTUAL,
					Sni:  routerCompatibleSNI(remote.ServiceName(), "istio-system", remote.ServicePort()),
				},
			},
		},
	}}

	for _, svc := range cf.importedServiceStore.From(remote) {
		dr := &v1alpha3.DestinationRule{
			ObjectMeta: createObjectMeta(svc.Name, svc.Namespace),
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
					Sni:  routerCompatibleSNI(svc.Name, svc.Namespace, port.Number),
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
			Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
		},
		Spec: istionetv1alpha3.Gateway{
			Selector: cf.cfg.MeshPeers.Local.Gateways.Ingress.Selector,
			Servers: []*istionetv1alpha3.Server{{
				Hosts: []string{},
				Port: &istionetv1alpha3.Port{
					Number:   cf.cfg.MeshPeers.Local.Gateways.Ingress.Port.Number,
					Name:     cf.cfg.MeshPeers.Local.Gateways.Ingress.Port.Name,
					Protocol: "TLS",
				},
				Tls: &istionetv1alpha3.ServerTLSSettings{
					Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
				},
			}},
		},
	}

	hosts := []string{fmt.Sprintf("federation-discovery-service-%s.%s.svc.cluster.local", cf.cfg.MeshPeers.Local.Name, cf.namespace)}
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

// EnvoyFilters returns patches for SNI filters matching SNIs of exported services in federation ingress gateway.
// These patches add SNI compatible with https://datatracker.ietf.org/doc/html/rfc952 required by OpenShift Router.
// This function returns nil when the local ingress type is "istio".
func (cf *ConfigFactory) EnvoyFilters() []*v1alpha3.EnvoyFilter {
	if cf.cfg.MeshPeers.Local.IngressType != config.OpenShiftRouter {
		return nil
	}

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
				Name:      fmt.Sprintf("sni-%s-%s-%d", svcName, svcNamespace, port),
				Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
				Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
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
								Name: fmt.Sprintf("0.0.0.0_%d", cf.cfg.MeshPeers.Local.Gateways.Ingress.Port.Number),
								FilterChain: &istionetv1alpha3.EnvoyFilter_ListenerMatch_FilterChainMatch{
									Sni: fmt.Sprintf("outbound_.%d_._.%s.%s.svc.cluster.local", port, svcName, svcNamespace),
								},
							},
						},
					},
					Patch: &istionetv1alpha3.EnvoyFilter_Patch{
						Operation: istionetv1alpha3.EnvoyFilter_Patch_MERGE,
						Value:     buildPatchStruct(fmt.Sprintf(`{"filter_chain_match":{"server_names":["%s"]}}`, routerCompatibleSNI(svcName, svcNamespace, uint32(port)))),
					},
				}},
			},
		}
	}

	envoyFilters := []*v1alpha3.EnvoyFilter{
		createEnvoyFilter(fmt.Sprintf("federation-discovery-service-%s", cf.cfg.MeshPeers.Local.Name), "istio-system", 15080),
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
	remote := cf.cfg.MeshPeers.Remote

	if len(remote.Addresses) == 0 {
		return nil, nil
	}

	var resolution istionetv1alpha3.ServiceEntry_Resolution
	if networking.IsIP(cf.cfg.MeshPeers.Remote.Addresses[0]) {
		resolution = istionetv1alpha3.ServiceEntry_STATIC
	} else {
		resolution = istionetv1alpha3.ServiceEntry_DNS
	}

	serviceEntries := []*v1alpha3.ServiceEntry{cf.serviceEntryForRemoteFederationController()}
	for _, importedSvc := range cf.importedServiceStore.From(remote) {
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
					Name:      fmt.Sprintf("import-%s-%s", importedSvc.Name, importedSvc.Namespace),
					Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
					Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
				},
				Spec: istionetv1alpha3.ServiceEntry{
					Hosts: []string{fmt.Sprintf("%s.%s.svc.cluster.local", importedSvc.Name, importedSvc.Namespace)},
					Ports: ports,
					Endpoints: slices.Map(cf.cfg.MeshPeers.Remote.Addresses, func(addr string) *istionetv1alpha3.WorkloadEntry {
						return &istionetv1alpha3.WorkloadEntry{
							Address: addr,
							Labels:  maps.MergeCopy(importedSvc.Labels, map[string]string{"security.istio.io/tlsMode": "istio"}),
							Ports:   makePortsMap(importedSvc.Ports, cf.cfg.MeshPeers.Remote.GetPort()),
							Network: cf.cfg.MeshPeers.Remote.Network,
						}
					}),
					Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
					Resolution: resolution,
				},
			})
		}
	}
	return serviceEntries, nil
}

func (cf *ConfigFactory) WorkloadEntries() ([]*v1alpha3.WorkloadEntry, error) {
	remote := cf.cfg.MeshPeers.Remote

	var workloadEntries []*v1alpha3.WorkloadEntry

	for _, importedSvc := range cf.importedServiceStore.From(remote) {
		_, err := cf.serviceLister.Services(importedSvc.Namespace).Get(importedSvc.Name)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get Service %s/%s: %w", importedSvc.Name, importedSvc.Namespace, err)
			}
		} else {
			// Service already exists - create WorkloadEntries.
			for idx, ip := range networking.Resolve(cf.cfg.MeshPeers.Remote.Addresses...) {
				workloadEntries = append(workloadEntries, &v1alpha3.WorkloadEntry{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("import-%s-%d", importedSvc.Name, idx),
						Namespace: importedSvc.Namespace,
						Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
					},
					Spec: istionetv1alpha3.WorkloadEntry{
						Address: ip,
						Labels:  maps.MergeCopy(importedSvc.Labels, map[string]string{"security.istio.io/tlsMode": "istio"}),
						Ports:   makePortsMap(importedSvc.Ports, cf.cfg.MeshPeers.Remote.GetPort()),
						Network: cf.cfg.MeshPeers.Remote.Network,
					},
				})
			}
		}
	}
	return workloadEntries, nil
}

func (cf *ConfigFactory) serviceEntryForRemoteFederationController() *v1alpha3.ServiceEntry {
	remote := cf.cfg.MeshPeers.Remote
	se := &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remote.ServiceName(),
			Namespace: cf.cfg.MeshPeers.Local.ControlPlane.Namespace,
			Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
		},
		Spec: istionetv1alpha3.ServiceEntry{
			Hosts: []string{remote.ServiceFQDN()},
			Ports: []*istionetv1alpha3.ServicePort{{
				Name:     "grpc",
				Number:   remote.ServicePort(),
				Protocol: "GRPC",
			}},
			Endpoints: slices.Map(remote.Addresses, func(addr string) *istionetv1alpha3.WorkloadEntry {
				return &istionetv1alpha3.WorkloadEntry{
					Address: addr,
					Labels:  map[string]string{"security.istio.io/tlsMode": "istio"},
					Ports:   map[string]uint32{"grpc": remote.GetPort()},
					Network: remote.Network,
				}
			}),
			Location: istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
		},
	}
	if networking.IsIP(remote.Addresses[0]) {
		se.Spec.Resolution = istionetv1alpha3.ServiceEntry_STATIC
	} else {
		se.Spec.Resolution = istionetv1alpha3.ServiceEntry_DNS
	}
	return se
}

// routerCompatibleSNI returns SNI compatible with https://datatracker.ietf.org/doc/html/rfc952 required by OpenShift Router.
func routerCompatibleSNI(svcName, svcNs string, port uint32) string {
	return fmt.Sprintf("%s-%d.%s.svc.cluster.local", svcName, port, svcNs)
}

func makePortsMap(ports []*v1alpha1.ServicePort, remotePort uint32) map[string]uint32 {
	m := make(map[string]uint32, len(ports))
	for _, p := range ports {
		m[p.Name] = remotePort
	}
	return m
}
