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

var _ Reconciler = (*GatewayResourceReconciler)(nil)

type GatewayResourceReconciler struct {
	client kube.Client
	cf     *istio.ConfigFactory
}

func NewGatewayResourceReconciler(client kube.Client, cf *istio.ConfigFactory) *GatewayResourceReconciler {
	return &GatewayResourceReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *GatewayResourceReconciler) GetTypeUrl() string {
	return xds.GatewayTypeUrl
}

func (r *GatewayResourceReconciler) Reconcile(ctx context.Context) error {
	gw, err := r.cf.IngressGateway()
	if err != nil {
		return fmt.Errorf("error generating ingress gateway: %w", err)
	}

	kind := "Gateway"
	apiVersion := "networking.istio.io/v1alpha3"
	newGW, err := r.client.Istio().NetworkingV1alpha3().Gateways(gw.GetNamespace()).Apply(ctx, &networkingv1alpha3.GatewayApplyConfiguration{
		TypeMetaApplyConfiguration: applyconfigurationv1.TypeMetaApplyConfiguration{
			Kind:       &kind,
			APIVersion: &apiVersion,
		},
		ObjectMetaApplyConfiguration: &applyconfigurationv1.ObjectMetaApplyConfiguration{
			Name:      &gw.Name,
			Namespace: &gw.Namespace,
			Labels:    gw.Labels,
		},
		Spec:   &gw.Spec,
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
		return fmt.Errorf("error applying ingress gateway: %w", err)
	}
	log.Infof("Applied ingress gateway: %v", newGW)

	return nil
}
