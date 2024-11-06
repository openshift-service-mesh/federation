# Federation

A Kubernetes controller that utilizes MCP-over-XDS protocol to configure mesh-federation in Istio.

The API allows to export services to remote peers using label selectors. An exported service is available 
in the importing cluster without any additional routing configuration. Controllers use gRPC protocol to exchange
exported services, and cross-cluster connections between controllers are secured with Istio mTLS.

## Motivation

In this deployment model, independent meshes deployed in different clusters can connect services without configuring
access to the k8s api-server in remote clusters. This allows to achieve multi-cluster connectivity for meshes managed
by different teams in different clusters.

## Development

### Prerequisites
1. Go 1.22+
2. protoc 3.19.0+
3. protoc-gen-go v1.30.0+
4. protoc-get-golang-deepcopy

### Commands

1. Compile controller:
    ```shell
    make
    ```
1. Run unit tests:
    ```shell
    make test
    ```
1. Build image:
    ```shell
    HUB=quay.io/maistra-dev TAG=test make docker-build
    ```
1. Push image:
    ```shell
    HUB=quay.io/maistra-dev TAG=test make docker-push
    ```
1. Run e2e tests:
    ```shell
    make e2e
    ```
1. Run e2e tests with specific Istio version and custom controller image:
    ```shell
    HUB=quay.io/maistra-dev TAG=test ISTIO_VERSION=1.23.0 make e2e
    ```
1. Run specific test suites:
    ```shell
    TEST_SUITES="k8s mcp" make e2e
    ```
1. Customize federation controller image used in tests (`TAG` is ignored if `BUILD_TEST_IMAGE=true` or not set):
   ```shell
   BUILD_TEST_IMAGE=false TAG=0.1 make e2e
   ```
