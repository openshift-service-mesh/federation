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
	"context"
	"fmt"
	"time"

	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/util/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	Namespace namespace.Instance
)

// Cluster wraps Istio  representation of cluster under test
// adding reference to deployed applications and methods
// to interact with  the cluster for federation test cases.
type Cluster struct {
	cluster.Cluster
	ContextName string
	index       int
	Apps        echo.Instances
}

// InternalName is assigned by Istio Test Framework when loading KUBECONFIG
// passed through CLI flags. There is currently no way to alternate those.
// Names are following a pattern of `cluster-%d` in order of passed KUBECONFIGs.
func (c *Cluster) InternalName() string {
	return fmt.Sprintf("cluster-%d", c.index)
}

var Clusters = struct {
	East,
	West Cluster
}{
	East: Cluster{
		index:       0,
		ContextName: "east",
	},
	West: Cluster{
		index:       1,
		ContextName: "west",
	},
}

// Resolve returns configured clusters for given instance from istio.
func Resolve(c cluster.Cluster) *Cluster {
	resolvedCluster, found := clustersByName[c.Name()]
	if !found {
		panic("attempts to interact with unknown cluster [" + c.Name() + "]. check your configuration.")
	}
	resolvedCluster.Cluster = c

	return resolvedCluster
}

// clustersByName keeps reference to clusters used in testing by their internal
// names (cluster-%d).
var clustersByName = map[string]*Cluster{
	Clusters.East.InternalName(): &Clusters.East,
	Clusters.West.InternalName(): &Clusters.West,
}

const strictMTLS = `
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: default
spec:
  mtls:
    mode: STRICT
`

func EnsureStrictMutualTLS(ctx resource.Context) error {
	for _, c := range ctx.Clusters() {
		cc := Resolve(c)
		if err := retry.UntilSuccess(func() error {
			return c.ApplyYAMLContents("istio-system", strictMTLS)
		}, retry.Timeout(30*time.Second), retry.Delay(200*time.Millisecond)); err != nil {
			return fmt.Errorf("failed to apply peer authentication in cluster %s: %w", cc.ContextName, err)
		}
	}

	return nil
}

func RemoveServiceFromClusters(name string, ns namespace.Getter, targetClusters ...cluster.Cluster) func(t resource.Context) error {
	return func(ctx resource.Context) error {
		for _, targetCluster := range targetClusters {
			if err := targetCluster.Kube().CoreV1().Services(ns.Get().Name()).Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil {
				return fmt.Errorf("failed to delete Service %s/%s from cluster %s: %w", name, ns.Get().Name(), targetCluster.Name(), err)
			}
		}

		return nil
	}
}
