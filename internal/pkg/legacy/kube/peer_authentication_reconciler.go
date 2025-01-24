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

	securityv1beta1 "istio.io/api/security/v1beta1"
	typev1beta1 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	applyconfigurationv1 "istio.io/client-go/pkg/applyconfiguration/meta/v1"
	applyv1beta "istio.io/client-go/pkg/applyconfiguration/security/v1beta1"
	"istio.io/istio/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-service-mesh/federation/internal/pkg/legacy/xds"
)

var _ Reconciler = (*PeerAuthResourceReconciler)(nil)

type PeerAuthResourceReconciler struct {
	client    kube.Client
	namespace string
}

func NewPeerAuthResourceReconciler(client kube.Client, namespace string) *PeerAuthResourceReconciler {
	return &PeerAuthResourceReconciler{
		client:    client,
		namespace: namespace,
	}
}

func (r *PeerAuthResourceReconciler) GetTypeUrl() string {
	return xds.PeerAuthenticationTypeUrl
}

func (r *PeerAuthResourceReconciler) Reconcile(ctx context.Context) error {
	pa := &v1beta1.PeerAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fds-strict-mtls",
			Namespace: r.namespace,
			Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
		},
		Spec: securityv1beta1.PeerAuthentication{
			Selector: &typev1beta1.WorkloadSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "federation-controller",
				},
			},
			Mtls: &securityv1beta1.PeerAuthentication_MutualTLS{
				Mode: securityv1beta1.PeerAuthentication_MutualTLS_STRICT,
			},
		},
	}

	kind := "PeerAuthentication"
	apiVersion := "security.istio.io/v1beta1"
	newPA, err := r.client.Istio().SecurityV1beta1().PeerAuthentications(pa.GetNamespace()).Apply(ctx, &applyv1beta.PeerAuthenticationApplyConfiguration{
		TypeMetaApplyConfiguration: applyconfigurationv1.TypeMetaApplyConfiguration{
			Kind:       &kind,
			APIVersion: &apiVersion,
		},
		ObjectMetaApplyConfiguration: &applyconfigurationv1.ObjectMetaApplyConfiguration{
			Name:      &pa.Name,
			Namespace: &pa.Namespace,
			Labels:    pa.Labels,
		},
		Spec: &pa.Spec,
	}, metav1.ApplyOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: apiVersion,
		},
		Force:        true,
		FieldManager: "federation-controller",
	})
	if err != nil {
		return fmt.Errorf("error applying peer authentication: %w", err)
	}
	log.Infof("Applied peer authentication: %v", newPA)

	return nil
}
