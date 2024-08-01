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

