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

package meshfederation

import (
	"context"

	securityspecv1beta1 "istio.io/api/security/v1beta1"
	typev1beta1 "istio.io/api/type/v1beta1"
	securityv1beta1 "istio.io/client-go/pkg/apis/security/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcilePeerAuthentication(ctx context.Context) error {
	peerAuth := &securityv1beta1.PeerAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fds-strict-mtls",
			Namespace: r.namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, peerAuth, func() error {
		peerAuth.Spec = securityspecv1beta1.PeerAuthentication{
			Selector: &typev1beta1.WorkloadSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "federation-controller",
				},
			},
			Mtls: &securityspecv1beta1.PeerAuthentication_MutualTLS{
				Mode: securityspecv1beta1.PeerAuthentication_MutualTLS_STRICT,
			},
		}
		return controllerutil.SetControllerReference(r.instance, peerAuth, r.Scheme())
	})
	return err
}
