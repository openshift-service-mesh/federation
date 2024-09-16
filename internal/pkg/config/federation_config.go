package config

type Federation struct {
	MeshPeers          MeshPeers
	ExportedServiceSet ExportedServiceSet
	ImportedServiceSet ImportedServiceSet
}

func (f *Federation) GetLocalDataPlanePort() uint32 {
	if f.MeshPeers.Local != nil && f.MeshPeers.Local.Gateways != nil && f.MeshPeers.Local.Gateways.Ingress != nil && f.MeshPeers.Local.Gateways.Ingress.Ports != nil && f.MeshPeers.Local.Gateways.Ingress.Ports.DataPlane != 0 {
		return f.MeshPeers.Local.Gateways.Ingress.Ports.DataPlane
	}
	return defaultDataPlanePort
}

func (f *Federation) GetLocalDiscoveryPort() uint32 {
	if f.MeshPeers.Local != nil && f.MeshPeers.Local.Gateways != nil && f.MeshPeers.Local.Gateways.Ingress != nil && f.MeshPeers.Local.Gateways.Ingress.Ports != nil && f.MeshPeers.Local.Gateways.Ingress.Ports.Discovery != 0 {
		return f.MeshPeers.Local.Gateways.Ingress.Ports.Discovery
	}
	return defaultDiscoveryPort
}

func (f *Federation) GetRemoteGatewayDataPlanePort() uint32 {
	if f.MeshPeers.Remote.Ports == nil || f.MeshPeers.Remote.Ports.DataPlane == 0 {
		return defaultDataPlanePort
	}
	return f.MeshPeers.Remote.Ports.DataPlane
}
