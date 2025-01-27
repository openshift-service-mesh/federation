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

package federatedservice

import (
	"context"
	"fmt"
	"strings"

	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/istio/pkg/maps"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/networking"
)

func (r *Reconciler) updateServiceOrWorkloadEntries(ctx context.Context, federatedService v1alpha1.FederatedService, sourceMeshes []string) error {
	var resolution istionetv1alpha3.ServiceEntry_Resolution
	for _, remote := range r.remotes {
		if len(remote.Addresses) == 0 {
			continue
		}
		if networking.IsIP(remote.Addresses[0]) {
			resolution = istionetv1alpha3.ServiceEntry_STATIC
			// if at least one mesh requires STATIC mesh resolution, then we have to resolve addresses for all meshes
			// as we can use only one resolution type for all endpoints
			break
		} else {
			resolution = istionetv1alpha3.ServiceEntry_DNS
		}
	}

	var service corev1.Service
	if couldBeLocalService(federatedService.Spec.Host) {
		name, ns := getServiceNameAndNs(federatedService.Spec.Host)
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &service); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		}
	}

	if service.GetObjectMeta().GetName() != "" {
		return r.createOrUpdateWorkloadEntries(ctx, federatedService, sourceMeshes, service.Name, service.Namespace)
	} else {
		return r.createOrUpdateServiceEntries(ctx, federatedService, sourceMeshes, resolution)
	}
}

func (r *Reconciler) createOrUpdateServiceEntries(
	ctx context.Context, federatedService v1alpha1.FederatedService, sourceMeshes []string, resolution istionetv1alpha3.ServiceEntry_Resolution,
) error {
	var ports []*istionetv1alpha3.ServicePort
	for _, port := range federatedService.Spec.Ports {
		ports = append(ports, &istionetv1alpha3.ServicePort{
			Name:       port.Name,
			Number:     uint32(port.Number),
			Protocol:   port.Protocol,
			TargetPort: uint32(port.TargetPort),
		})
	}

	var allEndpoints []*istionetv1alpha3.WorkloadEntry
	for _, remote := range r.remotes {
		if !slices.Contains(sourceMeshes, remote.Name) {
			continue
		}
		// TODO: resolve addresses for all remotes if at least one remote requires static resolution
		endpoints := slices.Map(remote.Addresses, func(addr string) *istionetv1alpha3.WorkloadEntry {
			return &istionetv1alpha3.WorkloadEntry{
				Address: addr,
				Labels:  maps.MergeCopy(federatedService.Spec.Labels, map[string]string{"security.istio.io/tlsMode": "istio"}),
				Ports:   makePortsMap(federatedService.Spec.Ports, remote.GetPort()),
				Network: remote.Network,
			}
		})
		allEndpoints = append(allEndpoints, endpoints...)
	}

	serviceEntry := &v1alpha3.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      separateWithDash(federatedService.Spec.Host),
			Namespace: r.configNamespace,
		},
	}
	// TODO: delete stale SEs
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, serviceEntry, func() error {
		serviceEntry.Spec = istionetv1alpha3.ServiceEntry{
			Hosts:      []string{federatedService.Spec.Host},
			Ports:      ports,
			Endpoints:  allEndpoints,
			Location:   istionetv1alpha3.ServiceEntry_MESH_INTERNAL,
			Resolution: resolution,
		}
		return nil
	})
	return err
}

func (r *Reconciler) createOrUpdateWorkloadEntries(
	ctx context.Context, federatedService v1alpha1.FederatedService, sourceMeshes []string, svcName, svcNs string,
) error {
	for _, remote := range r.remotes {
		if !slices.Contains(sourceMeshes, remote.Name) {
			continue
		}
		for idx, ip := range networking.Resolve(remote.Addresses...) {
			workloadEntry := &v1alpha3.WorkloadEntry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("import-%s-%s-%d", remote.Name, svcName, idx),
					Namespace: svcNs,
				},
			}
			// TODO: delete stale WEs
			if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, workloadEntry, func() error {
				workloadEntry.Spec = istionetv1alpha3.WorkloadEntry{
					Address: ip,
					Labels:  maps.MergeCopy(federatedService.Spec.Labels, map[string]string{"security.istio.io/tlsMode": "istio"}),
					Ports:   makePortsMap(federatedService.Spec.Ports, remote.GetPort()),
					Network: remote.Network,
				}
				return nil
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func couldBeLocalService(host string) bool {
	domainLabels := strings.Split(host, ".")
	return len(domainLabels) == 5 && strings.HasSuffix(host, "svc.cluster.local")
}

func getServiceNameAndNs(hostname string) (string, string) {
	domainLabels := strings.Split(hostname, ".")
	return domainLabels[0], domainLabels[1]
}

func makePortsMap(ports []v1alpha1.Port, remotePort uint32) map[string]uint32 {
	m := make(map[string]uint32, len(ports))
	for _, p := range ports {
		m[p.Name] = remotePort
	}
	return m
}

func separateWithDash(hostname string) string {
	domainLabels := strings.Split(hostname, ".")
	return strings.Join(domainLabels, "-")
}
