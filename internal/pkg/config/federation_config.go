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
	Local Local `yaml:"local"`
	// TODO: This should be a list of Remote objects
	Remote Remote `yaml:"remote"`
}

type Local struct {
	ControlPlane ControlPlane `yaml:"controlPlane"`
	Gateways     Gateways     `yaml:"gateway"`
}

type Remote struct {
	Addresses []string      `yaml:"addresses"`
	Ports     *GatewayPorts `yaml:"ports"`
	Network   string        `yaml:"network"`
}

type ControlPlane struct {
	Namespace string `yaml:"namespace"`
}

type Gateways struct {
	Ingress LocalGateway `yaml:"ingress"`
}

type LocalGateway struct {
	Selector map[string]string `yaml:"selector"`
	Ports    *GatewayPorts     `yaml:"ports"`
}

type GatewayPorts struct {
	DataPlane uint32 `yaml:"dataPlane"`
	Discovery uint32 `yaml:"discovery"`
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
	Rules []Rules `yaml:"rules"`
}

func (s *ExportedServiceSet) GetLabelSelectors() []LabelSelectors {
	if len(s.Rules) == 0 {
		return []LabelSelectors{}
	}
	return s.Rules[0].LabelSelectors
}

type ImportedServiceSet struct {
	Rules []Rules `yaml:"rules"`
}

type Rules struct {
	Type           string           `yaml:"type"`
	LabelSelectors []LabelSelectors `yaml:"labelSelectors"`
}

type LabelSelectors struct {
	MatchLabels      map[string]string  `yaml:"matchLabels,omitempty"`
	MatchExpressions []MatchExpressions `yaml:"matchExpressions,omitempty"`
}

type MatchExpressions struct {
	Key      string   `yaml:"key"`
	Operator string   `yaml:"operator"`
	Values   []string `yaml:"values"`
}
