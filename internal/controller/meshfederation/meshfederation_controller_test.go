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
	"strconv"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/meta"

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

	AfterEach(func(ctx context.Context) {
		envTest.DeleteAll(ctx, testNs)
	})

	When("Using Istio Ingress Config", func() {

		It("should export services using defined label matcher", func(ctx context.Context) {
			// given
			services, _ := generateServices(testNsName, "hello", 5)

			for _, service := range services {
				_, errCreate := controllerutil.CreateOrUpdate(ctx, envTest.Client, service, func() error {
					// noop
					return nil
				})
				Expect(errCreate).ToNot(HaveOccurred())
			}

			// when
			meshFederation := createMeshFederation("local", testNsName)
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

			defer func() {
				cleanup := append([]k8sclient.Object(nil), meshFederation)
				for _, service := range services {
					cleanup = append(cleanup, service)
				}
				envTest.DeleteAll(ctx, cleanup...)
			}()

			// then
			Eventually(func(g Gomega, ctx context.Context) error {
				currentMeshFederation := &v1alpha1.MeshFederation{}
				if errGet := envTest.Get(ctx, k8sclient.ObjectKeyFromObject(meshFederation), currentMeshFederation); errGet != nil {
					return errGet
				}

				g.Expect(currentMeshFederation.Status.Conditions).To(
					ContainElement(WithTransform(extractStatusOf("MeshFederationReconciled"), Equal(metav1.ConditionTrue))),
					"Expects MeshFederationReconciled condition to have status True",
				)

				g.Expect(currentMeshFederation.Status.ExportedServices).To(ConsistOf(testNsName + "/hello-2"))

				envoyFilters := &v1alpha3.EnvoyFilterList{}
				if errList := envTest.List(ctx, envoyFilters); errList != nil {
					return errList
				}
				g.Expect(envoyFilters.Items).To(BeEmpty())

				routes := &routev1.RouteList{}
				if errList := envTest.List(ctx, routes); errList != nil {
					return errList
				}
				g.Expect(routes.Items).To(BeEmpty())

				return nil
			}).WithContext(ctx).
				Within(4 * time.Second).
				ProbeEvery(250 * time.Millisecond).
				Should(Succeed())

		})
	})

	When("Using Openshift Router as Ingress", func() {

		It("should export services using defined match expression matcher", func(ctx context.Context) {
			// given

			meshFederation := createMeshFederation("local", testNsName)
			// Defaulting is not working correctly yet, therefore explicit settings for the .spec
			// see: https://github.com/openshift-service-mesh/federation/pull/155
			meshFederation.Spec.Network = "west"
			meshFederation.Spec.ControlPlaneNamespace = testNsName
			meshFederation.Spec.IngressConfig.Type = "openshift-router"
			meshFederation.Spec.IngressConfig.GatewayConfig.Selector = map[string]string{
				"security.istio.io/tlsMode": "istio",
			}
			meshFederation.Spec.IngressConfig.GatewayConfig.PortConfig = v1alpha1.PortConfig{
				Name:   "tls-passthrough",
				Number: 15443,
			}
			meshFederation.Spec.ExportRules = &v1alpha1.ExportRules{
				ServiceSelectors: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"hello-1", "hello-4"},
						},
					},
				},
			}

			_, err := controllerutil.CreateOrUpdate(ctx, envTest.Client, meshFederation, func() error {
				return nil
			})
			Expect(err).ToNot(HaveOccurred())

			// when
			var services []*corev1.Service
			defer func() {
				cleanup := append([]k8sclient.Object(nil), meshFederation)
				for _, service := range services {
					cleanup = append(cleanup, service)
				}
				envTest.DeleteAll(ctx, cleanup...)
			}()

			services, _ = generateServices(testNsName, "hello", 5)

			for _, service := range services {
				_, errCreate := controllerutil.CreateOrUpdate(ctx, envTest.Client, service, func() error {
					// noop
					return nil
				})
				Expect(errCreate).ToNot(HaveOccurred())
			}

			// then
			Eventually(func(g Gomega, ctx context.Context) error {
				currentMeshFederation := &v1alpha1.MeshFederation{}
				if errGet := envTest.Get(ctx, k8sclient.ObjectKeyFromObject(meshFederation), currentMeshFederation); errGet != nil {
					return errGet
				}

				g.Expect(currentMeshFederation.Status.Conditions).To(
					ContainElement(WithTransform(extractStatusOf("MeshFederationReconciled"), Equal(metav1.ConditionTrue))),
					"Expects MeshFederationReconciled condition to have status True",
				)

				g.Expect(currentMeshFederation.Status.ExportedServices).To(ConsistOf(testNsName+"/hello-1", testNsName+"/hello-4"))

				envoyFilters := &v1alpha3.EnvoyFilterList{}
				if errList := envTest.List(ctx, envoyFilters); errList != nil {
					return errList
				}
				g.Expect(envoyFilters.Items).To(Not(BeEmpty()))

				routes := &routev1.RouteList{}
				if errList := envTest.List(ctx, routes); errList != nil {
					return errList
				}
				g.Expect(routes.Items).To(Not(BeEmpty()))

				return nil
			}).WithContext(ctx).
				Within(4 * time.Second).
				ProbeEvery(250 * time.Millisecond).
				Should(Succeed())

		})
	})

	// TODO: follow-up with more tests
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

func generateServices(ns, svcPrefix string, count int) ([]*corev1.Service, []string) {
	var services []*corev1.Service
	var names []string

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("%s-%s", svcPrefix, strconv.Itoa(i))
		names = append(names, ns+"/"+name)
		svc := createSvc(name, ns)
		meta.AddLabel(svc, "app", "hello-"+strconv.Itoa(i))
		services = append(services, svc)
	}

	return services, names
}

func createSvc(name, ns string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "hello",
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "tcp",
					Port:     42,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Name:     "udp",
					Port:     42,
					Protocol: corev1.ProtocolUDP,
				},
			},
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
