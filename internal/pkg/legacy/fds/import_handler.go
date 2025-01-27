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

package fds

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"
	fdsv1alpha1 "github.com/openshift-service-mesh/federation/internal/api/federation/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds/adsc"
)

var _ adsc.ResponseHandler = (*ImportedServiceHandler)(nil)

type ImportedServiceHandler struct {
	client          client.Client
	configNamespace string
}

func NewImportedServiceHandler(c client.Client, configNamespace string) *ImportedServiceHandler {
	return &ImportedServiceHandler{
		client:          c,
		configNamespace: configNamespace,
	}
}

func (h *ImportedServiceHandler) Handle(source string, resources []*anypb.Any) error {
	newFederatedServices := make([]*fdsv1alpha1.FederatedService, 0, len(resources))
	for _, res := range resources {
		federatedService := &fdsv1alpha1.FederatedService{}
		if err := proto.Unmarshal(res.Value, federatedService); err != nil {
			return fmt.Errorf("unable to unmarshal exported service: %w", err)
		}
		newFederatedServices = append(newFederatedServices, federatedService)
	}

	if err := h.cleanupStaleServices(context.Background(), source, newFederatedServices); err != nil {
		return err
	}

	for _, svc := range newFederatedServices {
		federatedService := &v1alpha1.FederatedService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", separateWithDash(svc.Hostname), source),
				Namespace: h.configNamespace,
				Labels:    map[string]string{"federation.openshift-service-mesh.io/source-mesh": source},
			},
		}
		_, err := controllerutil.CreateOrUpdate(context.Background(), h.client, federatedService, func() error {
			federatedService.Spec = v1alpha1.FederatedServiceSpec{
				Host:   svc.Hostname,
				Ports:  mapPorts(svc.Ports),
				Labels: svc.Labels,
			}
			return nil
		})
		return err
	}

	return nil
}

func (h *ImportedServiceHandler) cleanupStaleServices(
	ctx context.Context, sourceMesh string, newFederatedServices []*fdsv1alpha1.FederatedService,
) error {
	newHostnamesMap := make(map[string]struct{})
	for _, svc := range newFederatedServices {
		newHostnamesMap[svc.Hostname] = struct{}{}
	}

	var currentFederatedServices v1alpha1.FederatedServiceList
	if err := h.client.List(context.Background(), &currentFederatedServices, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{"federation.openshift-service-mesh.io/source-mesh": sourceMesh}),
	}); err != nil {
		return fmt.Errorf("unable to list federated services: %w", err)
	}

	for _, svc := range currentFederatedServices.Items {
		if _, exists := newHostnamesMap[svc.Spec.Host]; !exists {
			if err := h.client.Delete(ctx, &svc); err != nil {
				return fmt.Errorf("failed to delete FederatedService %s/%s: %w", svc.Namespace, svc.Name, err)
			}
		}
	}
	return nil
}

func separateWithDash(hostname string) string {
	domainLabels := strings.Split(hostname, ".")
	return strings.Join(domainLabels, "-")
}

func mapPorts(ports []*fdsv1alpha1.ServicePort) []v1alpha1.Port {
	var out []v1alpha1.Port
	for _, port := range ports {
		out = append(out, v1alpha1.Port{
			Name:       port.Name,
			Number:     int32(port.Number),
			Protocol:   port.Protocol,
			TargetPort: int32(port.TargetPort),
		})
	}
	return out
}
