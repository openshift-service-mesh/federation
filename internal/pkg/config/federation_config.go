package config

type Federation struct {
	MeshPeers          MeshPeers
	ExportedServiceSet ExportedServiceSet
	ImportedServiceSet ImportedServiceSet
}

func (f *Federation) GetLocalDataPlaneGatewayNamespace() string {
	if f.MeshPeers.Local.Gateways.DataPlane.Namespace == "" {
		return f.MeshPeers.Local.ControlPlane.Namespace
	}
	return f.MeshPeers.Local.Gateways.DataPlane.Namespace
}

func (f *Federation) GetLocalDataPlaneGatewayPort() uint32 {
	if f.MeshPeers.Local.Gateways.DataPlane.Port == 0 {
		return defaultDataPlanePort
	}
	return f.MeshPeers.Local.Gateways.DataPlane.Port
}

func (f *Federation) GetRemoteDataPlaneGatewayPort() uint32 {
	if f.MeshPeers.Remote.DataPlane.Port == 0 {
		return defaultDataPlanePort
	}
	return f.MeshPeers.Remote.DataPlane.Port
}
