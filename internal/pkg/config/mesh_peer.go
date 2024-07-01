package config

type MeshPeers struct {
	Spec Spec `yaml:"spec"`
}

type Ports struct {
	DataPlane int `yaml:"dataPlane"`
	Discovery int `yaml:"discovery"`
}

type Remote struct {
	Addresses []string `yaml:"addresses"`
	Ports     Ports    `yaml:"ports"`
}

type Spec struct {
	Remote Remote `yaml:"remote"`
}