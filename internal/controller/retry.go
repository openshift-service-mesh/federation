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

package controller

import (
	"context"
	"errors"

	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MutateFn is a function that allows defining custom
// logic for updating resource object.
type MutateFn[T client.Object] func(saved T)

// ClientCallFn defines what client.client operation on a given object should be performed.
type ClientCallFn[T client.Object] func(ctx context.Context, cli client.Client, obj T) error

// RetryUpdate attempts to update a specified Kubernetes resource and retries on conflict.
func RetryUpdate[T client.Object](ctx context.Context, cli client.Client, original T, mutate MutateFn[T], opts ...client.UpdateOption) (T, error) {
	updateObjFn := func(ctx context.Context, cli client.Client, obj T) error {
		return cli.Update(ctx, obj, opts...)
	}

	return retryCall[T](ctx, cli, original, mutate, updateObjFn)
}

// RetryStatusUpdate attempts to update status subresource of a specified Kubernetes resource and retries on conflict.
func RetryStatusUpdate[T client.Object](ctx context.Context, cli client.Client, original T, mutate MutateFn[T], opts ...client.SubResourceUpdateOption) (T, error) {
	updateStatusFn := func(ctx context.Context, cli client.Client, obj T) error {
		return cli.Status().Update(ctx, obj, opts...)
	}

	return retryCall[T](ctx, cli, original, mutate, updateStatusFn)
}

func retryCall[T client.Object](
	ctx context.Context,
	cli client.Client,
	original T,
	mutate MutateFn[T],
	updateFn ClientCallFn[T],
) (T, error) {

	current, ok := original.DeepCopyObject().(T)
	if !ok {
		var zero T
		return zero, errors.New("failed to deep copy object")
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := cli.Get(ctx, client.ObjectKeyFromObject(original), current); err != nil {
			return err
		}
		mutate(current)

		// Return the mutate error directly so the RetryOnConflict logic can identify conflicts
		return updateFn(ctx, cli, current)
	})

	return current, err
}
