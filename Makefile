PROJECT_DIR:=$(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
OUT_DIR:=out

export ISTIO_VERSION ?= 1.23.0

.PHONY: default
default: build test

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m\033[2m %s\033[0m\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build

EXTRA_BUILD_ARGS?=
.PHONY: build
build: proto add-license ## Builds the project
	go get $(PROJECT_DIR)/...
	go build -C $(PROJECT_DIR)/cmd/federation-controller -o $(PROJECT_DIR)/$(OUT_DIR)/federation-controller $(EXTRA_BUILD_ARGS)

.PHONY: test
test: build ## Runs tests
	go test $(PROJECT_DIR)/...

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

##@ Development

define local_tag
$(TAG)$(shell [ "$(USE_LOCAL_IMAGE)" = "true" ] && echo "-local")
endef

.PHONY: kind-clusters
kind-clusters: ## Provisions KinD clusters for local development or testing
	@local_tag=$(call local_tag); \
	$(MAKE) docker-build -e TAG=$$local_tag; \
	export TAG=$$local_tag; \
	$(PROJECT_DIR)/test/scripts/kind_provisioner.sh

.PHONY: e2e
TEST_SUITES ?= remote_ip remote_dns_name spire
e2e: tools kind-clusters ## Runs end-to-end tests against KinD clusters
	@local_tag=$(call local_tag); \
	$(foreach suite, $(TEST_SUITES), \
		PATH=$(LOCALBIN):$$PATH \
		TAG=$$local_tag \
		go test -tags=integ -run TestTraffic $(PROJECT_DIR)/test/e2e/$(suite) \
			--istio.test.hub=docker.io/istio\
			--istio.test.tag=$(ISTIO_VERSION)\
			--istio.test.kube.config=$(PROJECT_DIR)/test/east.kubeconfig,$(PROJECT_DIR)/test/west.kubeconfig\
			--istio.test.kube.networkTopology=0:east-network,1:west-network\
			--istio.test.onlyWorkloads=standard;)

##@ Tooling

LOCALBIN := $(PROJECT_DIR)/bin
GOIMPORTS := $(LOCALBIN)/goimports
HELM := $(LOCALBIN)/helm
PROTOC := $(LOCALBIN)/protoc
PROTOC_GEN_GO := $(LOCALBIN)/protoc-gen-go
PROTOC_GEN_GRPC := $(LOCALBIN)/protoc-gen-go-grpc
PROTOC_GEN_DEEPCOPY := $(LOCALBIN)/protoc-gen-golang-deepcopy

$(LOCALBIN):
	@mkdir -p $(LOCALBIN)

$(GOIMPORTS): $(LOCALBIN)
	@GOBIN=$(LOCALBIN) go install -mod=readonly golang.org/x/tools/cmd/goimports@latest

$(HELM): $(LOCALBIN)
	@curl -sSL https://get.helm.sh/helm-v3.14.2-linux-amd64.tar.gz -o $(LOCALBIN)/helm.tar.gz
	@tar -xzf $(LOCALBIN)/helm.tar.gz -C $(LOCALBIN) --strip-components=1 linux-amd64/helm
	@rm -f $(LOCALBIN)/helm.tar.gz

$(PROTOC): $(LOCALBIN)
	@curl -sSL https://github.com/protocolbuffers/protobuf/releases/download/v21.12/protoc-21.12-linux-x86_64.zip -o $(LOCALBIN)/protoc.zip
	@python3 -c "import zipfile; z=zipfile.ZipFile('$(LOCALBIN)/protoc.zip'); z.extract('bin/protoc', '$(LOCALBIN)')"
	@mv $(LOCALBIN)/bin/protoc $(LOCALBIN)
	@rm -rf $(LOCALBIN)/bin
	@rm -f $(LOCALBIN)/protoc.zip
	@chmod +x $(PROTOC)

$(PROTOC_GEN_GO): $(LOCALBIN)
	@GOBIN=$(LOCALBIN) go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

$(PROTOC_GEN_GRPC): $(LOCALBIN)
	@GOBIN=$(LOCALBIN) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

$(PROTOC_GEN_DEEPCOPY): $(LOCALBIN)
	@GOBIN=$(LOCALBIN) go install istio.io/tools/cmd/protoc-gen-golang-deepcopy@latest

.PHONY: tools 
tools: $(GOIMPORTS) 
tools: $(HELM) 
tools: $(PROTOC) $(PROTOC_GEN_GO) $(PROTOC_GEN_GRPC) $(PROTOC_GEN_DEEPCOPY)
tools: ## Installs all required tools

.PHONY: clean
clean: 
	@rm -rf $(LOCALBIN) $(PROJECT_DIR)/$(OUT_DIR)

##@ Code Gen

PROTO_DIR=$(PROJECT_DIR)/api/proto/federation
API_GEN_DIR=$(PROJECT_DIR)/internal/api

.PHONY: proto
proto: $(PROTOC) $(PROTOC_GEN_GO) $(PROTOC_GEN_GRPC) $(PROTOC_GEN_DEEPCOPY) ## Generates Go files from protobuf-based API files
	@PATH=$(LOCALBIN):$$PATH $(PROTOC) --proto_path=$(PROTO_DIR) --go_out=$(API_GEN_DIR) --go-grpc_out=$(API_GEN_DIR) --golang-deepcopy_out=:$(API_GEN_DIR) $(PROTO_DIR)/**/*.proto


.PHONY: fix-imports
fix-imports: $(GOIMPORTS) ## Fixes imports
	$(GOIMPORTS) -local "github.com/openshift-service-mesh/federation" -w $(PROJECT_DIR)/

LICENSE_FILE := /tmp/license.txt
GO_FILES := $(shell find $(PROJECT_DIR)/ -name '*.go')

.PHONY: add-license
add-license: ## Adds license to all Golang files
	@echo "// Copyright Red Hat, Inc." > $(LICENSE_FILE)
	@echo "//" >> $(LICENSE_FILE)
	@echo "// Licensed under the Apache License, Version 2.0 (the "License");" >> $(LICENSE_FILE)
	@echo "// you may not use this file except in compliance with the License." >> $(LICENSE_FILE)
	@echo "// You may obtain a copy of the License at" >> $(LICENSE_FILE)
	@echo "//" >> $(LICENSE_FILE)
	@echo "//     http://www.apache.org/licenses/LICENSE-2.0" >> $(LICENSE_FILE)
	@echo "//" >> $(LICENSE_FILE)
	@echo "// Unless required by applicable law or agreed to in writing, software" >> $(LICENSE_FILE)
	@echo "// distributed under the License is distributed on an "AS IS" BASIS," >> $(LICENSE_FILE)
	@echo "// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied." >> $(LICENSE_FILE)
	@echo "// See the License for the specific language governing permissions and" >> $(LICENSE_FILE)
	@echo "// limitations under the License." >> $(LICENSE_FILE)
	@echo "" >> $(LICENSE_FILE)

	@for file in $(GO_FILES); do \
		if ! grep -q "Licensed under the Apache License" $$file; then \
			echo "Adding license to $$file"; \
			cat $(LICENSE_FILE) $$file > temp && mv temp $$file; \
		fi \
	done
	@rm -f $(LICENSE_FILE)
