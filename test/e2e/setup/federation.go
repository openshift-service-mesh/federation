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
	"fmt"

	"golang.org/x/sync/errgroup"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/scopes"
)

const meshFederationTemplate = `
apiVersion: federation.openshift-service-mesh.io/v1alpha1
kind: MeshFederation
metadata:
  name: %s
  namespace: istio-system
spec:
  ingress:
    type: istio
    gateway:
      selector:
        app: federation-ingress-gateway
      portConfig:
        name: tls-passthrough
        number: 15443
  export:
    serviceSelectors:
      matchLabels:
        export: "true"
`

func InstallOrUpgradeFederationControllers(options ...CtrlOption) resource.SetupFn {
	return func(ctx resource.Context) error {
		ctx.Cleanup(func() {
			for _, c := range ctx.Clusters() {
				cc := Resolve(c)
				if out, err := cc.UninstallFederationCtrl(); err != nil {
					scopes.Framework.Errorf("failed to uninstall federation controller (cluster=%s): %s: %v", cc.ContextName, out, err)
				}
			}
		})

		var g errgroup.Group
		for _, c := range ctx.Clusters() {
			localCluster := Resolve(c)
			remoteClusters := ctx.Clusters().Exclude(localCluster)
			g.Go(func() error {
				if out, err := localCluster.ConfigureFederationCtrl(remoteClusters, options...); err != nil {
					return fmt.Errorf("failed to upgrade federation controller (cluster=%s): %v, %w", localCluster.ContextName, string(out), err)
				}

				return nil
			})
		}

		return g.Wait()
	}
}

func CreateMeshFederationCR(ctx resource.Context) error {
	ctx.Cleanup(func() {
		for _, c := range ctx.Clusters() {
			localCluster := Resolve(c)
			if err := c.DeleteYAMLFiles("istio-system", fmt.Sprintf(meshFederationTemplate, localCluster.ContextName)); err != nil {
				scopes.Framework.Errorf("failed to delete mesh federation (cluster=%s): %v", localCluster.ContextName, err)
			}
		}
	})

	var g errgroup.Group
	for _, c := range ctx.Clusters() {
		localCluster := Resolve(c)
		g.Go(func() error {
			if err := c.ApplyYAMLContents("istio-system", fmt.Sprintf(meshFederationTemplate, localCluster.ContextName)); err != nil {
				return fmt.Errorf("failed to apply mesh federation (cluster=%s): %w", localCluster.ContextName, err)
			}
			return nil
		})
	}

	return g.Wait()
}
