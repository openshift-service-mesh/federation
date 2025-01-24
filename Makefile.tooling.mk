KIND := $(LOCALBIN)/kind
HELM := $(LOCALBIN)/helm
PROTOC := $(LOCALBIN)/protoc
PROTOC_GEN_GO := $(LOCALBIN)/protoc-gen-go
PROTOC_GEN_GRPC := $(LOCALBIN)/protoc-gen-go-grpc
PROTOC_GEN_DEEPCOPY := $(LOCALBIN)/protoc-gen-golang-deepcopy
CONTROLLER_GEN := $(LOCALBIN)/controller-gen
GCI := $(LOCALBIN)/gci

$(shell mkdir -p $(LOCALBIN))

$(GCI):
	@GOBIN=$(LOCALBIN) go install github.com/daixiang0/gci@v0.13.5

$(HELM):
	@curl -sSL https://get.helm.sh/helm-v3.14.2-linux-amd64.tar.gz -o $(LOCALBIN)/helm.tar.gz
	@tar -xzf $(LOCALBIN)/helm.tar.gz -C $(LOCALBIN) --strip-components=1 linux-amd64/helm
	@rm -f $(LOCALBIN)/helm.tar.gz

$(PROTOC):
	@curl -sSL https://github.com/protocolbuffers/protobuf/releases/download/v21.12/protoc-21.12-linux-x86_64.zip -o $(LOCALBIN)/protoc.zip
	@python3 -c "import zipfile; z=zipfile.ZipFile('$(LOCALBIN)/protoc.zip'); z.extract('bin/protoc', '$(LOCALBIN)')"
	@mv $(LOCALBIN)/bin/protoc $(LOCALBIN)
	@rm -rf $(LOCALBIN)/bin
	@rm -f $(LOCALBIN)/protoc.zip
	@chmod +x $(PROTOC)

$(PROTOC_GEN_GO):
	@GOBIN=$(LOCALBIN) go install -mod=readonly google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.0

$(PROTOC_GEN_GRPC):
	@GOBIN=$(LOCALBIN) go install -mod=readonly google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

$(PROTOC_GEN_DEEPCOPY):
	@GOBIN=$(LOCALBIN) go install -mod=readonly istio.io/tools/cmd/protoc-gen-golang-deepcopy@latest

$(KIND):
	@GOBIN=$(LOCALBIN) go install -mod=readonly sigs.k8s.io/kind@v0.26.0

CONTROLLER_TOOLS_VERSION ?= v0.16.4
$(CONTROLLER_GEN):
	GOBIN=$(LOCALBIN) go install -mod=readonly sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

