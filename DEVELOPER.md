## Development

### Prerequisites

To get started, ensure you have the following tools installed on your system:

#### Required

1. Go 1.22+
1. `make`

#### Recommended

1. A container runtime such as [Docker](https://www.docker.com/) **or** [Podman](https://podman.io/).
1. [KinD](https://kind.sigs.k8s.io/) for end-to-end tests.

> [!TIP]
> All other tools will be automatically downloaded to the local `./bin` directory when executing `make` targets.

### Commands

You can build the project with a single `make` command.

> [!TIP]
> Run `make help` to list all available targets.

Below are some commonly used targets with customizations

1. Build image with the custom tag in your own repository:
    ```shell
    make docker-build -e HUB=quay.io/maistra-dev -e TAG=test
    ```
1. Push image to your own repository and custom tag:
    ```shell
    make docker-push -e HUB=quay.io/maistra-dev -e TAG=test 
    ```
1. Run all E2E test suites:
    ```shell
    make e2e
    ```
1. Run specific test suite:
    ```shell
    make e2e -e TEST_SUITES="spire"
    ```
1. Run e2e tests against specific Istio version and custom controller image:
    ```shell
    make e2e -e HUB=quay.io/maistra-dev -e TAG=test -e ISTIO_VERSION=1.23.0
    ```

> [!TIP]
> Set the `USE_LOCAL_IMAGE` environment variable to `true` to push and use the locally built image in KinD clusters. 
