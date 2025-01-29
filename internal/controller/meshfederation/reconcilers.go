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
	"errors"
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	"google.golang.org/protobuf/types/known/structpb"
	istionetv1alpha3 "istio.io/api/networking/v1alpha3"
	securityv1beta1 "istio.io/api/security/v1beta1"
	typev1beta1 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	"istio.io/istio/pkg/util/protomarshal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"
)

type objReconciler[T client.Object, Spec any] struct {
	Obj         T
	DesiredSpec func() Spec
}

func PeerAuth(ctx context.Context, cl client.Client, meshFederation *v1alpha1.MeshFederation) (ctrl.Result, error) {
	desiredSpec := func() securityv1beta1.PeerAuthentication {
		return securityv1beta1.PeerAuthentication{
			Selector: &typev1beta1.WorkloadSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "federation-controller",
				},
			},
			Mtls: &securityv1beta1.PeerAuthentication_MutualTLS{
				Mode: securityv1beta1.PeerAuthentication_MutualTLS_STRICT,
			},
		}
	}

	peerAuth := &v1beta1.PeerAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fds-strict-mtls",
			Namespace: meshFederation.Spec.ControlPlaneNamespace,
			Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
		},
		Spec: desiredSpec(),
	}

	peerAuthReconciler := objReconciler[*v1beta1.PeerAuthentication, securityv1beta1.PeerAuthentication]{
		Obj: peerAuth,
		DesiredSpec: func() securityv1beta1.PeerAuthentication {
			return desiredSpec()
		},
	}

	pa := peerAuthReconciler.Obj
	_, err := ctrl.CreateOrUpdate(ctx, cl, peerAuthReconciler.Obj, func() error {
		pa.Spec = peerAuthReconciler.DesiredSpec()
		return controllerutil.SetControllerReference(meshFederation, pa, cl.Scheme())
	})

	return ctrl.Result{}, err
}

type EnvoyFilter struct {
	exportedServices *corev1.ServiceList
}

func (e EnvoyFilter) Reconcile(ctx context.Context, cl client.Client, meshFederation *v1alpha1.MeshFederation) (ctrl.Result, error) {

	desiredSpec := func(svcName, svcNamespace string, port int32) istionetv1alpha3.EnvoyFilter {
		buildPatchStruct := func(config string) *structpb.Struct {
			val := &structpb.Struct{}
			if err := protomarshal.UnmarshalString(config, val); err != nil {
				fmt.Printf("error unmarshalling envoyfilter config %q: %v", config, err)
			}
			return val
		}

		routerCompatibleSNI := func(svcName, svcNs string, port uint32) string {
			return fmt.Sprintf("%s-%d.%s.svc.cluster.local", svcName, port, svcNs)
		}

		return istionetv1alpha3.EnvoyFilter{
			WorkloadSelector: &istionetv1alpha3.WorkloadSelector{
				Labels: meshFederation.Spec.IngressConfig.GatewayConfig.Selector,
			},
			ConfigPatches: []*istionetv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{{
				ApplyTo: istionetv1alpha3.EnvoyFilter_FILTER_CHAIN,
				Match: &istionetv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
					ObjectTypes: &istionetv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
						Listener: &istionetv1alpha3.EnvoyFilter_ListenerMatch{
							Name: fmt.Sprintf("0.0.0.0_%d", meshFederation.Spec.IngressConfig.GatewayConfig.PortConfig.Number),
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
		}
	}

	envoyFilter := func(svcName, svcNamespace string, port int32) objReconciler[*v1alpha3.EnvoyFilter, istionetv1alpha3.EnvoyFilter] {
		result := &v1alpha3.EnvoyFilter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("sni-%s-%s-%d", svcName, svcNamespace, port),
				Namespace: meshFederation.Spec.ControlPlaneNamespace,
				Labels: map[string]string{
					"federation.openshift-service-mesh.io/peer": "todo",
				},
			},
		}

		result.Spec = desiredSpec(svcName, svcNamespace, port)

		return objReconciler[*v1alpha3.EnvoyFilter, istionetv1alpha3.EnvoyFilter]{
			Obj: result,
			DesiredSpec: func() istionetv1alpha3.EnvoyFilter {
				return desiredSpec(svcName, svcNamespace, port)
			},
		}
	}

	envoyFilters := []objReconciler[*v1alpha3.EnvoyFilter, istionetv1alpha3.EnvoyFilter]{envoyFilter(fmt.Sprintf("federation-discovery-service-%s", meshFederation.Name), "istio-system", 15080)}

	for _, svc := range e.exportedServices.Items {
		for _, port := range svc.Spec.Ports {
			envoyFilters = append(envoyFilters, envoyFilter(svc.Name, svc.Namespace, port.Port))
		}
	}

	var errs []error
	for _, desiredState := range envoyFilters {
		ef := desiredState.Obj
		_, err := ctrl.CreateOrUpdate(ctx, cl, ef, func() error {
			ef.Spec = desiredState.DesiredSpec()
			return controllerutil.SetControllerReference(meshFederation, ef, cl.Scheme())
		})

		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create or update envoy filter %s: %w", ef.Name, err))
		}
	}

	return ctrl.Result{}, errors.Join(errs...)
}

type RouteReconciler struct {
	exportedServices *corev1.ServiceList
}

func (r RouteReconciler) Reconcile(ctx context.Context, cl client.Client, meshFederation *v1alpha1.MeshFederation) (ctrl.Result, error) {
	desiredSpec := func(svcName, svcNamespace string, port int32) routev1.RouteSpec {
		return routev1.RouteSpec{
			Host: fmt.Sprintf("%s-%d.%s.svc.cluster.local", svcName, port, svcNamespace),
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: "federation-ingress-gateway",
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString(meshFederation.Spec.IngressConfig.GatewayConfig.PortConfig.Name),
			},
			TLS: &routev1.TLSConfig{
				Termination: routev1.TLSTerminationPassthrough,
			},
		}
	}

	createRoute := func(svcName, svcNamespace string, port int32) objReconciler[*routev1.Route, routev1.RouteSpec] {
		result := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s-%d-to-federation-ingress-gateway", svcName, svcNamespace, port),
				Namespace: meshFederation.Spec.ControlPlaneNamespace,
				Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
			},
		}

		result.Spec = desiredSpec(svcName, svcNamespace, port)

		return objReconciler[*routev1.Route, routev1.RouteSpec]{
			Obj: result,
			DesiredSpec: func() routev1.RouteSpec {
				return desiredSpec(svcName, svcNamespace, port)
			},
		}
	}

	routes := []objReconciler[*routev1.Route, routev1.RouteSpec]{
		createRoute(fmt.Sprintf("federation-discovery-service-%s", meshFederation.Name), "istio-system", 15080),
	}

	for _, svc := range r.exportedServices.Items {
		for _, port := range svc.Spec.Ports {
			routes = append(routes, createRoute(svc.Name, svc.Namespace, port.Port))
		}
	}

	var errs []error
	for _, route := range routes {
		rt := route.Obj
		_, err := ctrl.CreateOrUpdate(ctx, cl, rt, func() error {
			rt.Spec = route.DesiredSpec()
			return controllerutil.SetControllerReference(meshFederation, rt, cl.Scheme())
		})

		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create or update route %s: %w", rt.Name, err))
		}
	}

	return ctrl.Result{}, errors.Join(errs...)
}

type IngressGatewayReconciler struct {
	exportedServices *corev1.ServiceList
}

func (i IngressGatewayReconciler) Reconcile(ctx context.Context, cl client.Client, meshFederation *v1alpha1.MeshFederation) (ctrl.Result, error) {
	hosts := []string{fmt.Sprintf("federation-discovery-service-%s.%s.svc.cluster.local", meshFederation.Name, meshFederation.Namespace)}
	for _, svc := range i.exportedServices.Items {
		hosts = append(hosts, fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace))
	}

	desiredSpec := func() istionetv1alpha3.Gateway {
		return istionetv1alpha3.Gateway{
			Selector: meshFederation.Spec.IngressConfig.GatewayConfig.Selector,
			Servers: []*istionetv1alpha3.Server{{
				Hosts: hosts,
				Port: &istionetv1alpha3.Port{
					Number:   meshFederation.Spec.IngressConfig.GatewayConfig.PortConfig.Number,
					Name:     meshFederation.Spec.IngressConfig.GatewayConfig.PortConfig.Name,
					Protocol: "TLS",
				},
				Tls: &istionetv1alpha3.ServerTLSSettings{
					Mode: istionetv1alpha3.ServerTLSSettings_AUTO_PASSTHROUGH,
				},
			}},
		}
	}

	createGateway := func() objReconciler[*v1alpha3.Gateway, istionetv1alpha3.Gateway] {
		result := &v1alpha3.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "federation-ingress-gateway",
				Namespace: meshFederation.Spec.ControlPlaneNamespace,
				Labels:    map[string]string{"federation.openshift-service-mesh.io/peer": "todo"},
			},
		}

		result.Spec = desiredSpec()

		return objReconciler[*v1alpha3.Gateway, istionetv1alpha3.Gateway]{
			Obj: result,
			DesiredSpec: func() istionetv1alpha3.Gateway {
				return desiredSpec()
			},
		}
	}

	desiredGateway := createGateway()

	gateway := desiredGateway.Obj
	_, err := ctrl.CreateOrUpdate(ctx, cl, gateway, func() error {
		gateway.Spec = desiredGateway.DesiredSpec()
		return controllerutil.SetControllerReference(meshFederation, gateway, cl.Scheme())
	})

	return ctrl.Result{}, err
}
