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

const (
	defaultDataPlanePort = 15443
	defaultDiscoveryPort = 15080
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
	ControlPlane ControlPlane `json:"controlPlane"`
	Gateways     Gateways     `json:"gateway"`
}

type Remote struct {
	Addresses []string      `json:"addresses"`
	Ports     *GatewayPorts `json:"ports,omitempty"`
	Network   string        `json:"network"`
}

type ControlPlane struct {
	Namespace string `json:"namespace"`
}

type Gateways struct {
	Ingress LocalGateway `json:"ingress"`
}

type LocalGateway struct {
	Selector map[string]string `json:"selector"`
	Ports    *GatewayPorts     `json:"ports,omitempty"`
}

type GatewayPorts struct {
	DataPlane uint32 `json:"dataPlane"`
	Discovery uint32 `json:"discovery"`
}

func (g *GatewayPorts) GetDataPlanePort() uint32 {
	if g != nil && g.DataPlane != 0 {
		return g.DataPlane
	}
	return defaultDataPlanePort
}

func (g *GatewayPorts) GetDiscoveryPort() uint32 {
	if g != nil && g.Discovery != 0 {
		return g.Discovery
	}
	return defaultDiscoveryPort
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
