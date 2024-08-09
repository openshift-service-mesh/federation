//go:build integ
// +build integ

package e2e

import (
	"fmt"
	"testing"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/check"
	"istio.io/istio/pkg/test/framework/components/echo/common/ports"
	"istio.io/istio/pkg/test/framework/components/echo/match"
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
		a[0].CallOrFail(t, echo.CallOptions{
			Address: fmt.Sprintf("b.%s.svc.cluster.local", westApps.namespace.Name()),
			Port:    ports.HTTP,
			Check:   check.Status(200),
		})
	})
}
