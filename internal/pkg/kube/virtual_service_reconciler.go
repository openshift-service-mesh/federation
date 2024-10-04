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

package kube

import (
	"context"
	"fmt"

	applyconfigurationv1 "istio.io/client-go/pkg/applyconfiguration/meta/v1"
	networkingv1alpha3 "istio.io/client-go/pkg/applyconfiguration/networking/v1alpha3"
	"istio.io/istio/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

var _ Reconciler = (*VirtualServiceReconciler)(nil)

type VirtualServiceReconciler struct {
	client kube.Client
	cf     *istio.ConfigFactory
}

func NewVirtualServiceReconciler(client kube.Client, cf *istio.ConfigFactory) *VirtualServiceReconciler {
	return &VirtualServiceReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *VirtualServiceReconciler) GetTypeUrl() string {
	return xds.VirtualServiceTypeUrl
}

func (r *VirtualServiceReconciler) Reconcile(ctx context.Context) error {
	vs := r.cf.GetVirtualServices()

	kind := "VirtualService"
	apiVersion := "networking.istio.io/v1alpha3"
	newVS, err := r.client.Istio().NetworkingV1alpha3().VirtualServices(vs.GetNamespace()).Apply(ctx, &networkingv1alpha3.VirtualServiceApplyConfiguration{
		TypeMetaApplyConfiguration: applyconfigurationv1.TypeMetaApplyConfiguration{
			Kind:       &kind,
			APIVersion: &apiVersion,
		},
		ObjectMetaApplyConfiguration: &applyconfigurationv1.ObjectMetaApplyConfiguration{
			Name:      &vs.Name,
			Namespace: &vs.Namespace,
		},
		Spec:   &vs.Spec,
		Status: nil,
	}, metav1.ApplyOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: apiVersion,
		},
		Force:        true,
		FieldManager: "federation-controller",
	})
	if err != nil {
		return fmt.Errorf("error applying virtual service: %v", err)
	}
	log.Infof("Applied virtual service: %v", newVS)

	return nil
}
