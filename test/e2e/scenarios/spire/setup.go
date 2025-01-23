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
	"bytes"
	"fmt"
	"sync"
	"text/template"

	"golang.org/x/sync/errgroup"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/scopes"

	"github.com/openshift-service-mesh/federation/test"
	"github.com/openshift-service-mesh/federation/test/e2e/setup"
)

var spireComponents = []string{
	"daemonset/spire-spiffe-csi-driver",
	"statefulset/spire-server",
	"deployment/spire-spiffe-oidc-discovery-provider",
	"daemonset/spire-agent",
}

var clusterFederatedTrustDomainTmpl = `
apiVersion: spire.spiffe.io/v1alpha1
kind: ClusterFederatedTrustDomain
metadata:
  name: {{.remoteClusterName}}
spec:
  className: spire-spire
  trustDomain: {{.remoteClusterName}}.local
  bundleEndpointURL: https://{{.remoteSpireServerIP}}:8443
  bundleEndpointProfile:
    type: https_spiffe
    endpointSPIFFEID: spiffe://{{.remoteClusterName}}.local/spire/server
  trustDomainBundle: |-
{{.remoteTrustBundle | indent 4 }}
`

func installSpireCRDs(ctx resource.Context) error {
	var g errgroup.Group
	for _, c := range ctx.Clusters() {
		targetCluster := setup.Resolve(c)

		helmUpgradeCmd := targetCluster.Command("helm", "upgrade", "--install", "-n", "default",
			"spire-crds", "spire-crds", "--repo", "https://spiffe.github.io/helm-charts-hardened/", "--version", "0.5.0")

		g.Go(func() error {
			if out, err := helmUpgradeCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to upgrade federation controller (c=%s): %v: %w", targetCluster.ContextName, string(out), err)
			}

			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	ctx.Cleanup(func() {
		for _, c := range ctx.Clusters() {
			targetCluster := setup.Resolve(c)

			helmUninstallCmd := targetCluster.Command("helm", "uninstall", "spire-crds", "-n", "default")

			if out, err := helmUninstallCmd.CombinedOutput(); err != nil {
				scopes.Framework.Errorf("failed to uninstall federation controller (c=%s): %s: %w", targetCluster.ContextName, out, err)
			}
		}
	})
	return nil
}

func installSpire(ctx resource.Context) error {
	var g errgroup.Group
	for _, c := range ctx.Clusters() {
		targetCluster := setup.Resolve(c)
		helmUpgradeCmd := targetCluster.Command("helm", "upgrade", "--install",
			"spire", "spire", "--repo", "https://spiffe.github.io/helm-charts-hardened/",
			"-n", "spire", "--create-namespace",
			fmt.Sprintf("--values=%s/test/testdata/spire/%s.yaml", test.ProjectRoot(), targetCluster.ContextName),
			"--version", "0.24.0")
		g.Go(func() error {
			if out, err := helmUpgradeCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to upgrade federation controller (cluster=%s): %v: %w", targetCluster.ContextName, string(out), err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if err := waitUntilAllComponentsAreReady(ctx, "spire", spireComponents...); err != nil {
		return err
	}
	ctx.Cleanup(func() {
		for _, c := range ctx.Clusters() {
			targetCluster := setup.Resolve(c)
			helmUninstallCmd := targetCluster.Command("helm", "uninstall", "spire", "-n", "spire")
			if out, err := helmUninstallCmd.CombinedOutput(); err != nil {
				scopes.Framework.Errorf("failed to uninstall federation controller (cluster=%s): %s: %v", targetCluster.ContextName, out, err)
			}
		}
	})
	return nil
}

func waitUntilAllComponentsAreReady(ctx resource.Context, ns string, components ...string) error {
	var g errgroup.Group
	for _, c := range ctx.Clusters() {
		targetCluster := setup.Resolve(c)
		g.Go(func() error {
			for _, component := range components {
				rolloutStatusCmd := targetCluster.Command("kubectl", "rollout", "status", component, "-n", ns)
				if out, err := rolloutStatusCmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to wait for %s (cluster=%s): %v: %w", component, targetCluster.ContextName, string(out), err)
				}
			}
			return nil
		})
	}
	return g.Wait()
}

func enableTrustDomainFederation(ctx resource.Context) error {

	getTrustBundle := func(c cluster.Cluster) (string, error) {
		targetCluster := setup.Resolve(c)
		getBundleCmd := targetCluster.Command(
			"kubectl", "exec",
			"-c", "spire-server",
			"-n", "spire",
			"--stdin", "spire-server-0", "--",
			"spire-server", "bundle", "show", "-format", "spiffe",
		)

		remoteTrustBundle, err := getBundleCmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to get trust bundle (cluster=%s): %v: %w", targetCluster.ContextName, string(remoteTrustBundle), err)
		}

		return string(remoteTrustBundle), nil
	}

	federatedTrustDomain := func(c cluster.Cluster) (bytes.Buffer, error) {
		var clusterFederatedTrustDomain bytes.Buffer

		spireServerIP, err := setup.GetLoadBalancerIP(c, "spire-server", "spire")
		if err != nil {
			return clusterFederatedTrustDomain, err
		}

		trustBundle, err := getTrustBundle(c)
		if err != nil {
			return clusterFederatedTrustDomain, err
		}

		tmpl, err := template.New("trustBundleFederation").Funcs(template.FuncMap{
			"indent": test.Indent,
		}).Parse(clusterFederatedTrustDomainTmpl)
		if err != nil {
			return clusterFederatedTrustDomain, err
		}

		err = tmpl.Execute(&clusterFederatedTrustDomain, map[string]string{
			"remoteClusterName":   setup.Resolve(c).ContextName,
			"remoteSpireServerIP": spireServerIP,
			"remoteTrustBundle":   trustBundle,
		})

		return clusterFederatedTrustDomain, err
	}

	var federatedDomains sync.Map
	var trustedDomainGroup errgroup.Group
	for _, c := range ctx.Clusters() {
		trustedDomainGroup.Go(func() error {
			domain, err := federatedTrustDomain(c)
			if err != nil {
				return err
			}
			federatedDomains.Store(c.Name(), domain.String())

			return nil
		})
	}
	if err := trustedDomainGroup.Wait(); err != nil {
		return fmt.Errorf("failed creating federated trust domains")
	}

	var clusterApplyGroup errgroup.Group
	for _, c := range ctx.Clusters() {
		localCluster := setup.Resolve(c)
		remoteClusters := ctx.AllClusters().Exclude(localCluster)
		for _, remoteCluster := range remoteClusters {
			clusterApplyGroup.Go(func() error {
				value, ok := federatedDomains.Load(remoteCluster.Name())
				if !ok {
					return fmt.Errorf("failed to load ClusterFederatedTrustDomain (cluster=%s)", localCluster.Name())
				}

				if err := localCluster.ApplyYAMLContents("", value.(string)); err != nil {
					return fmt.Errorf("failed to apply ClusterFederatedTrustDomain (cluster=%s): %w", localCluster.Name(), err)
				}

				return nil
			})
		}
	}

	return clusterApplyGroup.Wait()
}
