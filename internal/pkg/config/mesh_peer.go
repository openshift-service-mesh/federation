package config

type MeshPeers struct {
	Spec Spec `yaml:"spec"`
}

type Ports struct {
	DataPlane uint32 `yaml:"dataPlane"`
	Discovery uint32 `yaml:"discovery"`
}

type Remote struct {
	Addresses []string `yaml:"addresses"`
	Ports     Ports    `yaml:"ports"`
	Network   string   `yaml:"network"`
	Locality  string   `yaml:"locality"`
}

type Spec struct {
	Remote Remote `yaml:"remote"`
}
