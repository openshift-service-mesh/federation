package spire

import (
	"bytes"
	"fmt"
	"github.com/openshift-service-mesh/federation/test/e2e/common"
	"golang.org/x/sync/errgroup"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/scopes"
	"os"
	"os/exec"
	"strings"
	"text/template"
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
		helmUpgradeCmd := exec.Command("helm", "upgrade", "--install",
			"spire-crds", "spire-crds", "--repo", "https://spiffe.github.io/helm-charts-hardened/", "--version", "0.5.0")
		helmUpgradeCmd.Env = os.Environ()
		helmUpgradeCmd.Env = append(helmUpgradeCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", common.RootDir, common.ClusterNames[idx]))
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
			helmUninstallCmd := exec.Command("helm", "uninstall", "spire-crds")
			helmUninstallCmd.Env = os.Environ()
			helmUninstallCmd.Env = append(helmUninstallCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", common.RootDir, common.ClusterNames[idx]))
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
		helmUpgradeCmd.Env = os.Environ()
		helmUpgradeCmd.Env = append(helmUpgradeCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", common.RootDir, common.ClusterNames[idx]))
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
			helmUninstallCmd.Env = os.Environ()
			helmUninstallCmd.Env = append(helmUninstallCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", common.RootDir, common.ClusterNames[idx]))
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
				rolloutStatusCmd.Env = os.Environ()
				rolloutStatusCmd.Env = append(rolloutStatusCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", common.RootDir, common.ClusterNames[idx]))
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
			getBundleCmd.Env = os.Environ()
			getBundleCmd.Env = append(getBundleCmd.Env, fmt.Sprintf("KUBECONFIG=%s/test/%s.kubeconfig", common.RootDir, common.ClusterNames[idx]))
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
