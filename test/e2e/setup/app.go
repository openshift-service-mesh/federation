// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integ
// +build integ

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
