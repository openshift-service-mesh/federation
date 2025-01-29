PROJECT_DIR:=$(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
OUT_DIR:=$(PROJECT_DIR)/out

.PHONY: default
default: build add-license fix-imports test

export ISTIO_VERSION ?= 1.23.0

## Required tooling.
LOCALBIN := $(PROJECT_DIR)/bin

include Makefile.tooling.mk
include Makefile.func.mk

PROTOBUF_API_DIR := $(PROJECT_DIR)/api/proto/federation
PROTOBUF_API_SRC := $(shell find $(PROTOBUF_API_DIR) -type f -name "*.proto")
API_GEN_DIR=$(PROJECT_DIR)/internal/api
PROTOBUF_GEN := $(shell find $(API_GEN_DIR) -type f -name "*.go")

CRD_SRC_DIR := $(PROJECT_DIR)/api/v1alpha1
CRD_SRC := $(shell find $(CRD_SRC_DIR) -type f -name "*.go")
CRD_GEN_DIR := $(PROJECT_DIR)/chart/crds
CRD_GEN := $(shell find $(CRD_GEN_DIR) -type f -name "*.yaml")

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m\033[2m %s\033[0m\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build

.PHONY: deps
deps: ## Downloads required dependencies
	go mod tidy
	go mod download

EXTRA_BUILD_ARGS?=
.PHONY: build
build: deps $(PROTOBUF_GEN) $(CRD_GEN) ## Builds the project
	go build -C $(PROJECT_DIR)/cmd/federation-controller -o $(OUT_DIR)/federation-controller $(EXTRA_BUILD_ARGS)

##@ Development

.PHONY: test
test: test-unit test-ctrl ## Runs unit and controller tests

.PHONY: test-unit
test-unit: build ## Runs unit tests
	go test $(PROJECT_DIR)/internal/pkg/...

ENVTEST_K8S_VERSION = 1.31 # refers to the version of kubebuilder assets to be downloaded by envtest binary.
test-ctrl: fetch-test-crds
test-ctrl: $(ENVTEST) $(GINKGO) ## Runs controller tests using k8s envtest
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" \
	$(GINKGO) $(PROJECT_DIR)/internal/controller/... -r \
		-vet=off \
		-coverprofile cover.out \
		--junit-report=ginkgo-test-results.xml ${args}

.PHONY: fetch-test-crds
fetch-test-crds: $(CONTROLLER_GEN)
	$(eval crd_folder = "$(PROJECT_DIR)/test/testdata/crds/external")
	@$(call fetch-external-crds,github.com/openshift/api,route/v1,$(crd_folder))
	@curl -s https://raw.githubusercontent.com/istio/istio/$(ISTIO_VERSION)/manifests/charts/base/crds/crd-all.gen.yaml > $(crd_folder)/istio.yaml


define local_tag
$(TAG)$(shell [ "$(USE_LOCAL_IMAGE)" = "true" ] && echo "-local")
endef

.PHONY: e2e
ALL_TEST_SUITES = remote_ip remote_dns_name spire
TEST_SUITES ?= remote_ip remote_dns_name spire
e2e: kind-clusters ## Runs end-to-end tests against KinD clusters
	@echo "Running '$(TEST_SUITES)'. Available test suites: '$(ALL_TEST_SUITES)'"
	@local_tag=$(call local_tag); \
	$(foreach suite, $(TEST_SUITES), \
		PATH=$(LOCALBIN):$$PATH \
		TAG=$$local_tag \
		go test -tags=integ -run TestTraffic $(PROJECT_DIR)/test/e2e/scenarios/$(suite) \
			--istio.test.hub=docker.io/istio\
			--istio.test.tag=$(ISTIO_VERSION)\
			--istio.test.kube.config=$(PROJECT_DIR)/test/east.kubeconfig,$(PROJECT_DIR)/test/west.kubeconfig,$(PROJECT_DIR)/test/central.kubeconfig\
			--istio.test.kube.networkTopology=0:east-network,1:west-network,2:central-network\
			--istio.test.onlyWorkloads=standard; \
	)

.PHONY: kind-clusters
kind-clusters: $(KIND) $(HELM) ## Provisions KinD clusters for local development or testing
	@local_tag=$(call local_tag); \
	$(MAKE) docker-build -e TAG=$$local_tag; \
	export TAG=$$local_tag; \
	PATH=$(LOCALBIN):$$PATH \
	$(PROJECT_DIR)/test/scripts/kind_provisioner.sh

##@ Containers

CONTAINER_CLI ?= docker
## Image settings need to be exported.
## KinD scripts rely on them to determine which images should be used
## and if they should be pushed to node's repository (USE_LOCAL_IMAGE).
export USE_LOCAL_IMAGE ?= true
export TAG ?= latest
export HUB ?= quay.io/maistra-dev

.PHONY: docker-build
docker-build: build ## Builds container image
	$(CONTAINER_CLI) build -t $(HUB)/federation-controller:$(TAG) -f $(PROJECT_DIR)/build/Dockerfile .

.PHONY: docker-push
docker-push: ## Pushes container image to the registry
	$(CONTAINER_CLI) push $(HUB)/federation-controller:$(TAG)

.PHONY: docker
docker: docker-build docker-push ## Combines build and push targets

## Code Gen

$(PROTOBUF_GEN): $(PROTOBUF_API_SRC) ## Generates Go files from protobuf-based API files
$(PROTOBUF_GEN): $(PROTOC) $(PROTOC_GEN_GO) $(PROTOC_GEN_GRPC) $(PROTOC_GEN_DEEPCOPY) # Required tools
	@PATH=$(LOCALBIN):$$PATH $(PROTOC) --proto_path=$(PROTOBUF_API_DIR) --go_out=$(API_GEN_DIR) --go-grpc_out=$(API_GEN_DIR) --golang-deepcopy_out=:$(API_GEN_DIR) $(PROTOBUF_API_DIR)/**/*.proto
	@$(MAKE) add-license
	@$(MAKE) fix-imports

$(CRD_GEN): $(CRD_SRC) ## Generates Kubernetes CRDs, controller-runtime artifacts and related manifests.
$(CRD_GEN): $(CONTROLLER_GEN) # Required tools
	$(CONTROLLER_GEN) paths="$(CRD_SRC_DIR)/..." \
		crd output:crd:artifacts:config="$(CRD_GEN_DIR)" \
		object:headerFile="$(LICENSE_FILE)"

##@ Misc

.PHONY: clean
clean: ## Purges local artifacts (e.g. binary, tools)
	@rm -rf $(LOCALBIN) $(OUT_DIR)

.PHONY: fix-imports
fix-imports: $(GCI) ## Fixes imports
	$(GCI) write $(PROJECT_DIR) \
		--section standard \
		--section default \
		--section "prefix(github.com/openshift-service-mesh/federation)" \
		--section blank \
		--section dot

LICENSE_FILE := $(PROJECT_DIR)/hack/boilerplate.go.txt
GO_FILES := $(shell find $(PROJECT_DIR)/ -name '*.go')

.PHONY: add-license
add-license: ## Adds license to all Golang files
	@for file in $(GO_FILES); do \
		if ! grep -q "Licensed under the Apache License" $$file; then \
			echo "Adding license to $$file"; \
			cat $(LICENSE_FILE) $$file > temp && mv temp $$file; \
		fi \
	done
