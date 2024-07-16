OUT := $(shell pwd)

.PHONY: build

build:
	go get ./...
	go build -C cmd/federation-controller -o "${OUT}/out/"

docker: build
	docker build -t quay.io/jewertow/federation-controller:latest -f build/Dockerfile .
