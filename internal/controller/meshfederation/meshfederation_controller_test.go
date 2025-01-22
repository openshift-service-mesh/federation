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

package meshfederation_test

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Configuring Mesh Federation in the cluster", func() {

	var (
		testNsName string
		testNs     *corev1.Namespace
	)

	BeforeEach(func(ctx context.Context) {
		testNsName = fmt.Sprintf("%s-%s", "mf-test", utilrand.String(8))

		testNs = &corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: testNsName,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, envTest.Client, testNs, func() error {
			return nil
		})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		envTest.DeleteAll(testNs)
	})

	When("new MeshFederation is created", func() {

		It("should update condition on successful reconciliation", func(ctx context.Context) {
			// given
			federationName := "test-west"
			meshFederation := createMeshFederation(federationName, testNsName)
			// Defaulting is not working correctly yet, therefore explicit settings for the .spec
			// see: https://github.com/openshift-service-mesh/federation/pull/155
			meshFederation.Spec.Network = "west"
			meshFederation.Spec.ControlPlaneNamespace = testNsName
			meshFederation.Spec.IngressConfig.Type = "istio"
			meshFederation.Spec.IngressConfig.GatewayConfig.Selector = map[string]string{
				"security.istio.io/tlsMode": "istio",
			}
			meshFederation.Spec.IngressConfig.GatewayConfig.PortConfig = v1alpha1.PortConfig{
				Name:   "tls-passthrough",
				Number: 15443,
			}
			meshFederation.Spec.ExportRules = &v1alpha1.ExportRules{
				ServiceSelectors: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "hello-2",
					},
				},
			}

			// when
			_, err := controllerutil.CreateOrUpdate(ctx, envTest.Client, meshFederation, func() error {
				return nil
			})
			Expect(err).ToNot(HaveOccurred())

			// then
			Eventually(func(g Gomega, ctx context.Context) error {
				currentMeshFederation := createMeshFederation(federationName, testNsName)
				if errGet := envTest.Get(ctx, k8sclient.ObjectKeyFromObject(currentMeshFederation), currentMeshFederation); errGet != nil {
					return errGet
				}

				g.Expect(currentMeshFederation.Status.Conditions).To(
					ContainElement(WithTransform(extractStatusOf("MeshFederationReconciled"), Equal(metav1.ConditionTrue))),
					"Expects MeshFederationReconciled condition to have status True",
				)

				return nil
			}).WithContext(ctx).
				Within(4 * time.Second).
				ProbeEvery(250 * time.Millisecond).
				Should(Succeed())
		})

	})

})

// createMeshFederation initializes MeshFederation struct with basic metadata.
func createMeshFederation(name, nsName string) *v1alpha1.MeshFederation {
	return &v1alpha1.MeshFederation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1alpha1",
			Kind:       "federation.openshift-service-mesh.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: nsName,
		},
	}
}

func extractStatusOf(reason string) func(c metav1.Condition) metav1.ConditionStatus {
	return func(c metav1.Condition) metav1.ConditionStatus {
		if c.Reason == reason {
			return c.Status
		}

		return metav1.ConditionStatus(fmt.Sprintf("ErrNotFound[%s]", reason))
	}
}
