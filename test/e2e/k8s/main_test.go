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

package k8s

import (
	"testing"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/namespace"

	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/test/e2e/common"
)

func TestMain(m *testing.M) {
	framework.
		NewSuite(m).
		RequireMinClusters(2).
		RequireMaxClusters(2).
		Setup(common.CreateNamespace).
		Setup(common.CreateCACertsSecret).
		Setup(common.DeployControlPlanes("k8s")).
		Setup(common.InstallOrUpgradeFederationControllers(true, config.ConfigModeK8s, false)).
		Setup(namespace.Setup(&common.AppNs, namespace.Config{Prefix: "app", Inject: true})).
		// a - client
		// b - service available in east and west clusters - covers importing with WorkloadEntry
		// c - service available only in west cluster - covers importing with ServiceEntry
		Setup(common.DeployApps(&common.EastApps, common.EastClusterName, namespace.Future(&common.AppNs), "a", "b")).
		Setup(common.DeployApps(&common.WestApps, common.WestClusterName, namespace.Future(&common.AppNs), "b", "c")).
		// c must be removed from the east cluster, because we want to test importing a service
		// that exists only in the remote cluster.
		Setup(common.RemoveServiceFromClusters("c", namespace.Future(&common.AppNs), common.EastClusterName)).
		Run()
}

func TestTraffic(t *testing.T) {
	framework.NewTest(t).Run(func(ctx framework.TestContext) {
		common.RunTrafficTests(t, ctx)
	})
}
