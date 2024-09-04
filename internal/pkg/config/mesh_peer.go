package config

const (
	defaultDataPlanePort = 15443
	defaultDiscoveryPort = 15020
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
	DataPlane *LocalDataPlaneGateway `yaml:"dataPlane"`
}

type LocalDataPlaneGateway struct {
	Namespace string            `yaml:"namespace"`
	Port      uint32            `yaml:"port"`
	Selector  map[string]string `yaml:"selector"`
}

func (l *Local) GetDataPlaneGatewayNamespace() string {
	if l.Gateways.DataPlane.Namespace == "" {
		return l.ControlPlane.Namespace
	}
	return l.Gateways.DataPlane.Namespace
}

func (g *LocalDataPlaneGateway) GetPort() uint32 {
	if g.Port == 0 {
		return defaultDataPlanePort
	}
	return g.Port
}

type Remote struct {
	DataPlane DataPlane `yaml:"dataPlane"`
	Discovery Discovery `yaml:"discovery"`
	Network   string    `yaml:"network"`
}

// TODO: unify DataPlane and Discovery
type DataPlane struct {
	Addresses []string `yaml:"addresses"`
	Port      *uint32  `yaml:"port"`
}

func (d *DataPlane) GetPort() uint32 {
	if d.Port != nil {
		return *d.Port
	}
	return defaultDataPlanePort
}

type Discovery struct {
	Addresses []string `yaml:"addresses"`
	Port      *uint32  `yaml:"port"`
}

func (d *Discovery) GetPort() uint32 {
	if d.Port != nil {
		return *d.Port
	}
	return defaultDiscoveryPort
}
