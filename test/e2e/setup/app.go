package setup

import (
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/namespace"
)

func DeployEcho(suite framework.Suite, opts ...DeployOption) {
	suite.Setup(namespace.Setup(&Namespace, namespace.Config{Prefix: "app", Inject: true})).
		// a - client app deployed only in east cluster
		// b - service deployed in all clusters - covers importing with WorkloadEntry
		// c - service deployed in central and west clusters - covers importing with ServiceEntry
		Setup(Clusters.East.DeployEcho(namespace.Future(&Namespace), "a", opts...)).
		Setup(Clusters.East.DeployEcho(namespace.Future(&Namespace), "b", opts...)).
		Setup(Clusters.West.DeployEcho(namespace.Future(&Namespace), "b", opts...)).
		Setup(Clusters.West.DeployEcho(namespace.Future(&Namespace), "c", opts...)).
		Setup(Clusters.Central.DeployEcho(namespace.Future(&Namespace), "b", opts...)).
		Setup(Clusters.Central.DeployEcho(namespace.Future(&Namespace), "c", opts...)).
		// c must be removed from east cluster, because we want to test importing a service
		// that exists only in the remote clusters.
		Setup(RemoveServiceFromClusters("c", namespace.Future(&Namespace), &Clusters.East))
}
