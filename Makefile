OUT := $(shell pwd)

.PHONY: build

export TAG ?= latest

build:
	go get ./...
	go build -C cmd/federation-controller -o "${OUT}/out/"

docker: build
	docker build -t quay.io/jewertow/federation-controller:$(TAG) -f build/Dockerfile .
