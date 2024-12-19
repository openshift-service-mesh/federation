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
    TEST_SUITES="spire" make e2e
    ```
1. Customize federation controller image used in tests (`TAG` is ignored if `USE_LOCAL_IMAGE=true` or not set):
   ```shell
   USE_LOCAL_IMAGE=false HUB=quay.io/maistra-dev TAG=0.1 make e2e
   ```
