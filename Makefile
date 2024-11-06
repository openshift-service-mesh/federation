OUT := $(shell pwd)

.PHONY: default
default: build test

export TAG ?= latest
export HUB ?= quay.io/maistra-dev
export ISTIO_VERSION ?= 1.23.0
export USE_LOCAL_IMAGE ?= true

.PHONY: build
build:
	go get ./...
	go build -C cmd/federation-controller -o "${OUT}/out/"

.PHONY: test
test: build
	go test ./...

CONTAINER_CLI ?= docker

.PHONY: docker-build
docker-build: build
	$(CONTAINER_CLI) build -t $(HUB)/federation-controller:$(TAG) -f build/Dockerfile .

.PHONY: docker-push
docker-push:
	$(CONTAINER_CLI) push $(HUB)/federation-controller:$(TAG)

.PHONY: docker
docker: docker-build docker-push

PROTO_DIR=api/proto/federation
OUT_DIR=internal/api

.PHONY: proto
proto:
	protoc --proto_path=$(PROTO_DIR) --go_out=$(OUT_DIR) --go-grpc_out=$(OUT_DIR) --golang-deepcopy_out=:$(OUT_DIR) $(PROTO_DIR)/**/*.proto

.PHONY: kind-clusters
kind-clusters: build-test-image
	bash test/scripts/kind_provisioner.sh $(ISTIO_VERSION)

.PHONY: build-test-image
build-test-image:
ifeq ($(USE_LOCAL_IMAGE), true)
	$(MAKE) docker-build -e TAG=test
endif

.PHONY: e2e
TEST_SUITES ?= mcp k8s
ifeq ($(USE_LOCAL_IMAGE),true)
	TEST_TAG := test
else
	TEST_TAG := $(TAG)
endif
e2e: build-test-image kind-clusters
	@$(foreach suite, $(TEST_SUITES), \
		TAG=$(TEST_TAG) go test -tags=integ -run TestTraffic ./test/e2e/$(suite) \
			--istio.test.hub=docker.io/istio\
			--istio.test.tag=$(ISTIO_VERSION)\
			--istio.test.kube.config=$(shell pwd)/test/east.kubeconfig,$(shell pwd)/test/west.kubeconfig\
			--istio.test.kube.networkTopology=0:east-network,1:west-network\
			--istio.test.onlyWorkloads=standard;)

.PHONY: fix-imports
fix-imports:
	goimports -local "github.com/openshift-service-mesh/federation" -w .

LICENSE_FILE := /tmp/license.txt
GO_FILES := $(shell find . -name '*.go')

.PHONY: add-license
add-license:
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
