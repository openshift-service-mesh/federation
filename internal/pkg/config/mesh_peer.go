package config

const (
	defaultDataPlanePort = 15443
	defaultDiscoveryPort = 15080
)

type MeshPeers struct {
	Local *Local `yaml:"local"`
	// TODO: This should be a list of Remote objects
	Remote Remote `yaml:"remote"`
}

type Local struct {
	ControlPlane *ControlPlane `yaml:"controlPlane"`
	Gateways     *Gateways     `yaml:"gateway"`
}

type ControlPlane struct {
	Namespace string `yaml:"namespace"`
}

type Gateways struct {
	Ingress *LocalGateway `yaml:"ingress"`
}

type LocalGateway struct {
	Selector map[string]string `yaml:"selector"`
	Ports    *GatewayPorts     `yaml:"ports"`
}

type GatewayPorts struct {
	DataPlane uint32 `yaml:"dataPlane"`
	Discovery uint32 `yaml:"discovery"`
}

type Remote struct {
	Addresses []string      `yaml:"addresses"`
	Ports     *GatewayPorts `yaml:"ports"`
	Network   string        `yaml:"network"`
}
