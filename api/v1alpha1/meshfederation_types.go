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

// Run "make build" to regenerate code after modifying this file

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func init() {
	SchemeBuilder.Register(&MeshFederation{}, &MeshFederationList{})
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced

// MeshFederation is the Schema for the meshfederations API.
type MeshFederation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MeshFederationSpec   `json:"spec,omitempty"`
	Status MeshFederationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MeshFederationList contains a list of MeshFederation.
type MeshFederationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MeshFederation `json:"items"`
}

// MeshFederationSpec defines the desired state of MeshFederation.
type MeshFederationSpec struct {
	// Network name used by Istio for load balancing
	// +kubebuilder:validation:Required
	Network string `json:"network"`

	// +kubebuilder:default:=cluster.local
	TrustDomain string `json:"trustDomain"`

	// Namespace used to create mesh-wide resources
	// +kubebuilder:default:=istio-system
	ControlPlaneNamespace string `json:"controlPlaneNamespace"`

	// TODO: CRD proposal states "If no ingress is specified, it means the controller supports only single network topology". However, some config, such as gateway/port config, seems to be required.
	// Config specifying ingress type and ingress gateway config
	// +kubebuilder:validation:Required
	IngressConfig IngressConfig `json:"ingress"`

	// Selects the K8s Services to export to all remote meshes.
	// An empty export object matches all Services in all namespaces.
	// A null export rules object matches no Services.
	// +kubebuilder:validation:Optional
	ExportRules *ExportRules `json:"export,omitempty"`
}

// MeshFederationStatus defines the observed state of MeshFederation.
type MeshFederationStatus struct {
	// Conditions describes the state of the MeshFederation resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type PortConfig struct {
	// TODO: Needs clarification: This was marked as optional in the CRD proposal, but the comment states it cannot be empty
	// Port name of the ingress gateway Service.
	// This is relevant only when the ingress type is openshift-router, but it cannot be empty
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Port of the ingress gateway Service
	// +kubebuilder:validation:Required
	Number uint32 `json:"number"`
}

type GatewayConfig struct {
	// Ingress gateway selector specifies to which workloads Gateway configurations will be applied.
	// +kubebuilder:validation:MinProperties=1
	Selector map[string]string `json:"selector"`

	// Specifies the port name and port number of the ingress gateway service
	// +kubebuilder:validation:Required
	PortConfig PortConfig `json:"portConfig"`
}

type IngressConfig struct {
	// Local ingress type specifies how to expose exported services.
	// Currently, only two types are supported: istio and openshift-router.
	// If "istio" is set, then the controller assumes that the Service associated with federation ingress gateway
	// is LoadBalancer or NodePort and is directly accessible for remote peers, and then it only creates
	// an auto-passthrough Gateway to expose exported Services.
	// When "openshift-router" is enabled, then the controller creates also OpenShift Routes and applies EnvoyFilters
	// to customize the SNI filter in the auto-passthrough Gateway, because the default SNI DNAT format used by Istio
	// is not supported by OpenShift Router.
	// +kubebuilder:default:=istio
	// +kubebuilder:validation:Enum=istio;openshift-router
	Type string `json:"type"`

	// Specifies the selector and port config of the ingress gateway
	// +kubebuilder:validation:Required
	GatewayConfig GatewayConfig `json:"gateway,omitempty"`
}

type ExportRules struct {
	// ServiceSelectors is a label query over K8s Services in all namespaces.
	// The result of matchLabels and matchExpressions are ANDed.
	// An empty service selector matches all Services.
	// A null service selector matches no Services.
	ServiceSelectors *metav1.LabelSelector `json:"serviceSelectors,omitempty"`
}
