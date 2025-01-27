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

package meshfederation_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/openshift-service-mesh/federation/internal/controller"
	"github.com/openshift-service-mesh/federation/internal/controller/meshfederation"
	"github.com/openshift-service-mesh/federation/test/k8senvtest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var envTest *k8senvtest.Client
var cancelFunc context.CancelFunc

func TestControllers(t *testing.T) {
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.TimeEncoderOfLayout(time.RFC3339),
	}
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseFlagOptions(&opts)))

	RegisterFailHandler(Fail)
	RunSpecs(t, "Controllers Integration Test Suite")
}

var _ = SynchronizedBeforeSuite(func(ctx context.Context) {
	newMeshFederationCtrl := func(cl client.Client) controller.Reconciler {
		return meshfederation.NewReconciler(cl)
	}
	envTest, cancelFunc = k8senvtest.StartWithControllers(GinkgoT(), newMeshFederationCtrl)
}, func() {})

var _ = SynchronizedAfterSuite(func() {}, func() {
	By("Tearing down the test environment")
	cancelFunc()
	Expect(envTest.Stop()).To(Succeed())
})
