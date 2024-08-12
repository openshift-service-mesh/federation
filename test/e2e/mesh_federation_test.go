//go:build integ
// +build integ

package e2e

import (
	"context"
	"fmt"
	"testing"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/check"
	"istio.io/istio/pkg/test/framework/components/echo/common/ports"
	"istio.io/istio/pkg/test/framework/components/echo/match"
	"istio.io/istio/pkg/test/util/retry"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const strictMTLS = `
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: default
spec:
  mtls:
    mode: STRICT
`

func TestMeshFederation(t *testing.T) {
	framework.NewTest(t).Run(func(ctx framework.TestContext) {
		// TODO: move to TestMain
		for _, c := range ctx.Clusters() {
			if err := c.ApplyYAMLContents("istio-system", strictMTLS); err != nil {
				t.Errorf("failed to apply peer authentication in cluster %s: %v", c.Name(), err)
			}
		}

		a := match.ServiceName(echo.NamespacedName{Name: "a", Namespace: eastApps.namespace}).GetMatches(eastApps.apps)
		if len(a) == 0 {
			ctx.Fatalf("failed to find a match for a")
		}
		if err := exportService(ctx.Clusters().GetByName(westClusterName), "b", westApps.namespace.Name()); err != nil {
			t.Errorf("failed to export service b in cluster %s: %v", westClusterName, err)
		}

		a[0].CallOrFail(t, echo.CallOptions{
			Address: fmt.Sprintf("b.%s.svc.cluster.local", westApps.namespace.Name()),
			Port:    ports.HTTP,
			Check:   check.Status(200),
		})
	})
}

func exportService(c cluster.Cluster, svcName, svcNs string) error {
	if err := retry.UntilSuccess(func() error {
		svc, err := c.Kube().CoreV1().Services(svcNs).Get(context.TODO(), svcName, v1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get service %s/%s: %v", svcNs, svcName, err)
		}
		svc.Labels["export-service"] = "true"
		_, err = c.Kube().CoreV1().Services(svcNs).Update(context.TODO(), svc, v1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update service %s/%s: %v", svcNs, svcName, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to export service %s/%s: %v", svcNs, svcName, err)
	}
	return nil
}
