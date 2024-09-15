package config

type Federation struct {
	MeshPeers          MeshPeers
	ExportedServiceSet ExportedServiceSet
	ImportedServiceSet ImportedServiceSet
}

func (f *Federation) GetLocalIngressGatewayNamespace() string {
	if f.MeshPeers.Local.Gateways.Ingress.Namespace == "" {
		return f.MeshPeers.Local.ControlPlane.Namespace
	}
	return f.MeshPeers.Local.Gateways.Ingress.Namespace
}

func (f *Federation) GetLocalDataPlanePort() uint32 {
	if f.MeshPeers.Local.Gateways.Ingress.Ports.DataPlane == 0 {
		return defaultDataPlanePort
	}
	return f.MeshPeers.Local.Gateways.Ingress.Ports.DataPlane
}

func (f *Federation) GetLocalDiscoveryPort() uint32 {
	if f.MeshPeers.Local.Gateways.Ingress.Ports.Discovery == 0 {
		return defaultDiscoveryPort
	}
	return f.MeshPeers.Local.Gateways.Ingress.Ports.Discovery
}

func (f *Federation) GetRemoteDataPlaneGatewayPort() uint32 {
	if f.MeshPeers.Remote.DataPlane.Port == 0 {
		return defaultDataPlanePort
	}
	return f.MeshPeers.Remote.DataPlane.Port
}
