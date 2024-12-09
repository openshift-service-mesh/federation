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

package config

import "fmt"

const (
	defaultGatewayPort = 15443
)

type Federation struct {
	MeshPeers          MeshPeers
	ExportedServiceSet ExportedServiceSet
	ImportedServiceSet ImportedServiceSet
}

type MeshPeers struct {
	Local Local `json:"local"`
	// TODO: This should be a list of Remote objects
	Remote Remote `json:"remote"`
}

type Local struct {
	Name         string       `json:"name"`
	ControlPlane ControlPlane `json:"controlPlane"`
	Gateways     Gateways     `json:"gateways"`
	IngressType  IngressType  `json:"ingressType"`
}

type Remote struct {
	Name        string      `json:"name"`
	Addresses   []string    `json:"addresses"`
	IngressType IngressType `json:"ingressType"`
	Port        *uint32     `json:"port,omitempty"`
	Network     string      `json:"network"`
}

func (r *Remote) ServiceName() string {
	return fmt.Sprintf("federation-discovery-service-%s", r.Name)
}

func (r *Remote) ServiceFQDN() string {
	return fmt.Sprintf("%s.istio-system.svc.cluster.local", r.ServiceName())
}

func (r *Remote) ServicePort() uint32 {
	return 15080
}

func (r *Remote) GetPort() uint32 {
	if r != nil && r.Port != nil {
		return *r.Port
	}
	return defaultGatewayPort
}

type ControlPlane struct {
	Namespace string `json:"namespace"`
}

type Gateways struct {
	Ingress LocalGateway `json:"ingress"`
}

type LocalGateway struct {
	Selector map[string]string `json:"selector"`
	Port     *GatewayPort      `json:"port,omitempty"`
}

type GatewayPort struct {
	Name   string `json:"name"`
	Number uint32 `json:"number"`
}

type GatewayPorts struct {
	DataPlane uint32 `json:"dataPlane"`
	Discovery uint32 `json:"discovery"`
}

type ExportedServiceSet struct {
	Rules []Rules `json:"rules"`
}

func (s *ExportedServiceSet) GetLabelSelectors() []LabelSelectors {
	if len(s.Rules) == 0 {
		return []LabelSelectors{}
	}
	return s.Rules[0].LabelSelectors
}

type ImportedServiceSet struct {
	Rules []Rules `json:"rules"`
}

type Rules struct {
	Type           string           `json:"type"`
	LabelSelectors []LabelSelectors `json:"labelSelectors"`
}

type LabelSelectors struct {
	MatchLabels      map[string]string  `json:"matchLabels,omitempty"`
	MatchExpressions []MatchExpressions `json:"matchExpressions,omitempty"`
}

type MatchExpressions struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

type IngressType string

const (
	Istio           IngressType = "istio"
	OpenShiftRouter IngressType = "openshift-router"
)
