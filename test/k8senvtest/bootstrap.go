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

package k8senvtest

import (
	"context"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift-service-mesh/federation/internal/controller"
	"github.com/openshift-service-mesh/federation/test"
)

type TestReporter interface {
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
}

type CtrlCreate func(cl client.Client) controller.Reconciler

func StartWithControllers(t TestReporter, createCtrls ...CtrlCreate) (*Client, context.CancelFunc) {
	// The context passed to Process 1, which is invoked before all parallel nodes are started by Ginkgo,
	// is terminated when this function exits. As a result, this context is unsuitable for use with
	// manager/controllers that need to be available for the entire duration of the test suite.
	// To address this, a new cancellable context must be created to ensure it remains active
	// throughout the test suite.
	ctx, cancel := context.WithCancel(context.TODO())

	testScheme := runtime.NewScheme()
	controller.MustAddToScheme(testScheme)
	utilruntime.Must(apiextv1.AddToScheme(testScheme))

	return Configure(
		WithCRDs(
			filepath.Join(test.ProjectRoot(), "chart", "crds"),                        // Project APIs
			filepath.Join(test.ProjectRoot(), "test", "testdata", "crds", "external"), // External CRDs used in testing
		),
		WithScheme(testScheme),
	).WithRecoverFunc(ginkgo.GinkgoRecover).WithControllers(createCtrls...).
		Start(ctx, t), cancel
}
