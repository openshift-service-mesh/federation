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

package remote_ip

import (
	"testing"

	"istio.io/istio/pkg/test/framework"

	"github.com/openshift-service-mesh/federation/test/e2e"
	"github.com/openshift-service-mesh/federation/test/e2e/setup"
)

func TestMain(m *testing.M) {
	suite := framework.
		NewSuite(m).
		RequireMinClusters(3).
		RequireMaxClusters(3).
		Setup(setup.RecreateControlPlaneNamespace).
		Setup(setup.CreateCACertsSecret).
		Setup(setup.DeployControlPlanes("k8s")).
		Setup(setup.InstallOrUpgradeFederationControllers()).
		Setup(setup.EnsureStrictMutualTLS)

	setup.DeployEcho(suite)
	suite.Run()
}

func TestTraffic(t *testing.T) {
	framework.NewTest(t).Run(func(ctx framework.TestContext) {
		e2e.RunTrafficTests(t, ctx)
	})
}
