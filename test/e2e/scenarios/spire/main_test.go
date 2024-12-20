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

package spire

import (
	"testing"

	"github.com/openshift-service-mesh/federation/test/e2e"
	"github.com/openshift-service-mesh/federation/test/e2e/setup"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/namespace"
)

func TestMain(m *testing.M) {
	framework.
		NewSuite(m).
		// TODO(multi-peer): two clusters
		RequireMinClusters(3).
		RequireMaxClusters(3).
		Setup(installSpireCRDs).
		Setup(installSpire).
		Setup(enableTrustDomainFederation).
		Setup(setup.DeployControlPlanes("spire")).
		Setup(setup.InstallOrUpgradeFederationControllers(setup.WithSpire{})).
		Setup(namespace.Setup(&setup.Namespace, namespace.Config{Prefix: "app", Inject: true})).
		Setup(setup.Clusters.East.DeployEcho(namespace.Future(&setup.Namespace), "a", setup.WithSpire{})).
		Setup(setup.Clusters.East.DeployEcho(namespace.Future(&setup.Namespace), "b", setup.WithSpire{})).
		Setup(setup.Clusters.West.DeployEcho(namespace.Future(&setup.Namespace), "b", setup.WithSpire{})).
		Setup(setup.Clusters.West.DeployEcho(namespace.Future(&setup.Namespace), "c", setup.WithSpire{})).
		Setup(setup.Clusters.Central.DeployEcho(namespace.Future(&setup.Namespace), "b", setup.WithSpire{})).
		Setup(setup.Clusters.Central.DeployEcho(namespace.Future(&setup.Namespace), "d", setup.WithSpire{})).
		// c and d must be removed from other clusters, because we want to test importing a service
		// that exists only in the remote cluster.
		Setup(setup.RemoveServiceFromClusters("c", namespace.Future(&setup.Namespace), &setup.Clusters.East, &setup.Clusters.Central)).
		Setup(setup.RemoveServiceFromClusters("d", namespace.Future(&setup.Namespace), &setup.Clusters.East, &setup.Clusters.West)).
		Setup(setup.EnsureStrictMutualTLS).
		Run()
}

func TestTraffic(t *testing.T) {
	framework.NewTest(t).Run(func(ctx framework.TestContext) {
		e2e.RunTrafficTests(t, ctx)
	})
}
