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
	"os/exec"
	"strings"
	"text/template"

	"golang.org/x/sync/errgroup"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/scopes"

	"github.com/openshift-service-mesh/federation/test/e2e/common"
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
	for idx := range ctx.Clusters() {
		helmUpgradeCmd := exec.Command("helm", "upgrade", "--install", "-n", "default",
			"spire-crds", "spire-crds", "--repo", "https://spiffe.github.io/helm-charts-hardened/", "--version", "0.5.0")
		common.SetEnvAndKubeConfigPath(helmUpgradeCmd, idx)
		g.Go(func() error {
			if out, err := helmUpgradeCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to upgrade federation controller (cluster=%s): %v: %w", common.ClusterNames[idx], string(out), err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	ctx.Cleanup(func() {
		for idx := range ctx.Clusters() {
			helmUninstallCmd := exec.Command("helm", "uninstall", "spire-crds", "-n", "default")
			common.SetEnvAndKubeConfigPath(helmUninstallCmd, idx)
			if out, err := helmUninstallCmd.CombinedOutput(); err != nil {
				scopes.Framework.Errorf("failed to uninstall federation controller (cluster=%s): %s: %w", common.ClusterNames[idx], out, err)
			}
		}
	})
	return nil
}

func installSpire(ctx resource.Context) error {
	var g errgroup.Group
	for idx := range ctx.Clusters() {
		helmUpgradeCmd := exec.Command("helm", "upgrade", "--install",
			"spire", "spire", "--repo", "https://spiffe.github.io/helm-charts-hardened/",
			"-n", "spire", "--create-namespace",
			fmt.Sprintf("--values=%s/examples/spire/%s/values.yaml", common.RootDir, common.ClusterNames[idx]),
			"--version", "0.24.0")
		common.SetEnvAndKubeConfigPath(helmUpgradeCmd, idx)
		g.Go(func() error {
			if out, err := helmUpgradeCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to upgrade federation controller (cluster=%s): %v: %w", common.ClusterNames[idx], string(out), err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if err := waitUntilAllComponentsAreReady(ctx); err != nil {
		return err
	}
	ctx.Cleanup(func() {
		for idx := range ctx.Clusters() {
			helmUninstallCmd := exec.Command("helm", "uninstall", "spire", "-n", "spire")
			common.SetEnvAndKubeConfigPath(helmUninstallCmd, idx)
			if out, err := helmUninstallCmd.CombinedOutput(); err != nil {
				scopes.Framework.Errorf("failed to uninstall federation controller (cluster=%s): %s: %v", common.ClusterNames[idx], out, err)
			}
		}
	})
	return nil
}

func waitUntilAllComponentsAreReady(ctx resource.Context) error {
	var g errgroup.Group
	for idx := range ctx.Clusters() {
		idx := idx
		g.Go(func() error {
			for _, component := range spireComponents {
				rolloutStatusCmd := exec.Command("kubectl", "rollout", "status", component, "-n", "spire")
				common.SetEnvAndKubeConfigPath(rolloutStatusCmd, idx)
				if out, err := rolloutStatusCmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to wait for %s (cluster=%s): %v: %w", component, common.ClusterNames[idx], string(out), err)
				}
			}
			return nil
		})
	}
	return g.Wait()
}

func enableTrustDomainFederation(ctx resource.Context) error {
	getRemoteClusterName := func(ctx resource.Context, localCluster cluster.Cluster, localClusterName string) string {
		for idx, remoteCluster := range ctx.Clusters() {
			if localCluster.Name() == remoteCluster.Name() {
				continue
			}
			return common.ClusterNames[idx]
		}
		// this should never happen
		panic("cluster name not found")
	}
	getRemoteSpireServerIP := func(ctx resource.Context, localCluster cluster.Cluster, localClusterName string) (string, error) {
		for idx, remoteCluster := range ctx.Clusters() {
			if localCluster.Name() == remoteCluster.Name() {
				continue
			}
			var err error
			remoteSpireServerIP, err := common.GetLoadBalancerIP(remoteCluster, "spire-server", "spire")
			if err != nil {
				return "", fmt.Errorf("failed to get spire server IP (cluster=%s)", common.ClusterNames[idx])
			}
			return remoteSpireServerIP, nil
		}
		return "", fmt.Errorf("no spire server IP found (cluster=%s)", localClusterName)
	}
	getRemoteTrustBundle := func(ctx resource.Context, localCluster cluster.Cluster, localClusterName string) (string, error) {
		for idx, remoteCluster := range ctx.Clusters() {
			if localCluster.Name() == remoteCluster.Name() {
				continue
			}
			getBundleCmd := exec.Command(
				"kubectl", "exec",
				"-c", "spire-server",
				"-n", "spire",
				"--stdin", "spire-server-0", "--",
				"spire-server", "bundle", "show", "-format", "spiffe",
			)
			common.SetEnvAndKubeConfigPath(getBundleCmd, idx)
			remoteTrustBundle, err := getBundleCmd.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("failed to get trust bundle (cluster=%s): %v: %w", common.ClusterNames[idx], string(remoteTrustBundle), err)
			}
			return string(remoteTrustBundle), nil
		}
		return "", fmt.Errorf("no trust bundle found (cluster=%s)", localClusterName)
	}
	var g errgroup.Group
	for idx, localCluster := range ctx.Clusters() {
		localCluster := localCluster
		localClusterName := common.ClusterNames[idx]
		g.Go(func() error {
			remoteClusterName := getRemoteClusterName(ctx, localCluster, localClusterName)
			remoteSpireServerIP, err := getRemoteSpireServerIP(ctx, localCluster, localClusterName)
			if err != nil {
				return err
			}
			remoteTrustBundle, err := getRemoteTrustBundle(ctx, localCluster, localClusterName)
			if err != nil {
				return err
			}
			tmpl, err := template.New("trustBundleFederation").Funcs(template.FuncMap{
				"indent": indent,
			}).Parse(clusterFederatedTrustDomainTmpl)
			if err != nil {
				return err
			}
			var clusterFederatedTrustDomain bytes.Buffer
			err = tmpl.Execute(&clusterFederatedTrustDomain, map[string]string{
				"remoteClusterName":   remoteClusterName,
				"remoteSpireServerIP": remoteSpireServerIP,
				"remoteTrustBundle":   remoteTrustBundle,
			})
			if err != nil {
				return fmt.Errorf("failed to generate ClusterFederatedTrustDomain (cluster=%s): %w", localClusterName, err)
			}
			if err := localCluster.ApplyYAMLContents("", clusterFederatedTrustDomain.String()); err != nil {
				return fmt.Errorf("failed to apply ClusterFederatedTrustDomain (cluster=%s): %w", localClusterName, err)
			}
			return nil
		})
	}
	return g.Wait()
}

func indent(spaces int, text string) string {
	prefix := strings.Repeat(" ", spaces)
	return prefix + strings.ReplaceAll(text, "\n", "\n"+prefix)
}
