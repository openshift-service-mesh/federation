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

	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/scopes"
)

func InstallOrUpgradeFederationControllers(options ...CtrlOption) resource.SetupFn {
	return func(ctx resource.Context) error {
		for _, c := range ctx.Clusters() {
			localCluster := Resolve(c)
			remoteClusters := ctx.Clusters().Exclude(localCluster)
			if out, err := localCluster.ConfigureFederationCtrl(remoteClusters, options...); err != nil {
				return fmt.Errorf("failed to upgrade federation controller (cluster=%s): %v, %w", localCluster.ContextName, string(out), err)
			}
		}

		ctx.Cleanup(func() {
			for _, c := range ctx.Clusters() {
				cc := Resolve(c)
				if out, err := cc.UninstallFederationCtrl(); err != nil {
					scopes.Framework.Errorf("failed to uninstall federation controller (cluster=%s): %s: %v", cc.ContextName, out, err)
				}
			}
		})

		return nil
	}
}
