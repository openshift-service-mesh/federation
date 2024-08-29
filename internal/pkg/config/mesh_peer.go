package config

type MeshPeers struct {
	Remote Remote `yaml:"remote"`
}

type Remote struct {
	DataPlane DataPlane `yaml:"dataPlane"`
	Discovery Discovery `yaml:"discovery"`
	Network   string    `yaml:"network"`
}

// TODO: unify DataPlane and Discovery
type DataPlane struct {
	Addresses []string `yaml:"addresses"`
	Port      uint32   `yaml:"port"`
}

type Discovery struct {
	Addresses []string `yaml:"addresses"`
	Port      uint32   `yaml:"port"`
}
