OUT := $(shell pwd)

.PHONY: default
default: build test

export TAG ?= latest
export HUB ?= quay.io/jewertow
export ISTIO_VERSION ?= 1.23.0

.PHONY: build
build:
	go get ./...
	go build -C cmd/federation-controller -o "${OUT}/out/"

.PHONY: test
test:
	go test ./...

.PHONY: docker
docker: build
	docker build -t $(HUB)/federation-controller:$(TAG) -f build/Dockerfile .

PROTO_DIR=api/proto/federation
OUT_DIR=internal/api

.PHONY: proto
proto:
	protoc --proto_path=$(PROTO_DIR) --go_out=$(OUT_DIR) --go-grpc_out=$(OUT_DIR) --golang-deepcopy_out=:$(OUT_DIR) $(PROTO_DIR)/**/*.proto

.PHONY: gen-istio-manifests
gen-istio-manifests:
	bash test/scripts/generate_istio_manifests.sh $(ISTIO_VERSION)

.PHONY: kind-clusters
kind-clusters:
	bash test/scripts/kind_provisioner.sh $(ISTIO_VERSION)

.PHONY: e2e-test
e2e-test:
	go test -tags=integ -run TestTraffic ./test/e2e \
		--istio.test.hub=docker.io/istio\
		--istio.test.tag=$(ISTIO_VERSION)\
		--istio.test.kube.config=$(shell pwd)/test/east.kubeconfig,$(shell pwd)/test/west.kubeconfig\
		--istio.test.kube.networkTopology=0:east-network,1:west-network\
		--istio.test.onlyWorkloads=standard

.PHONY: e2e
e2e: kind-clusters e2e-test
