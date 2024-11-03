package spire

import (
	"fmt"
	"github.com/openshift-service-mesh/federation/test/e2e/common"
	"golang.org/x/sync/errgroup"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/scopes"
	"os"
	"os/exec"
)

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
			fmt.Sprintf("--values=%s/test/testdata/spire/%s.yaml", common.RootDir, common.ClusterNames[idx]),
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
