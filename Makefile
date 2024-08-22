OUT := $(shell pwd)

.PHONY: build

export TAG ?= latest

build:
	go get ./...
	go build -C cmd/federation-controller -o "${OUT}/out/"

docker: build
	docker build -t quay.io/jewertow/federation-controller:$(TAG) -f build/Dockerfile .

PROTO_DIR=api/proto/federation
OUT_DIR=internal/api

.PHONY: proto
proto:
	protoc --proto_path=$(PROTO_DIR) --go_out=$(OUT_DIR) --go-grpc_out=$(OUT_DIR) $(PROTO_DIR)/**/*.proto

kind-clusters:
	bash test/scripts/kind_provisioner.sh

e2e-test:
	go test -tags=integ -run TestTraffic ./test/e2e \
		--istio.test.kube.config=$(shell pwd)/test/east.kubeconfig,$(shell pwd)/test/west.kubeconfig\
		--istio.test.kube.networkTopology=0:east-network,1:west-network\
		--istio.test.onlyWorkloads=standard

e2e: kind-clusters e2e-test
