# Tool versions
GOIMPORT_VER           := latest
CONTROLLER_GEN_VERSION := v0.17.1
BUF_VERSION            := v1.50.0
PROTOC_GEN_GO_GRPC_VER := v1.5.1
PROTOC_GEN_GO_VER      := v1.36.5
UPX_VER 			   := 4.2.4

# Tool fully qualified paths
TOOLS_DIR := $(PWD)/out/tools
GOIMPORTS_FQP := $(TOOLS_DIR)/goimports-$(GOIMPORT_VER)
CONTROLLER_GEN_FQP := $(TOOLS_DIR)/controller-gen-$(CONTROLLER_GEN_VERSION)
BUF_FQP := $(TOOLS_DIR)/buf-$(BUF_VERSION)
PROTOC_GEN_GO_GRPC_FQP := $(TOOLS_DIR)/protoc-gen-go-grpc-$(PROTOC_GEN_GO_GRPC_VER)
PROTOC_GEN_GO_FQP := $(TOOLS_DIR)/protoc-gen-go-$(PROTOC_GEN_GO_VER)
UPX_FQP := $(TOOLS_DIR)/upx-$(UPX_VER)-$(LOCAL_ARCH)

$(GOIMPORTS_FQP):
	GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@$(GOIMPORT_VER)
	@mv $(TOOLS_DIR)/goimports $(GOIMPORTS_FQP)

$(CONTROLLER_GEN_FQP):
	GOBIN=$(TOOLS_DIR) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
	@mv $(TOOLS_DIR)/controller-gen $(CONTROLLER_GEN_FQP)

$(PROTOC_GEN_GO_GRPC_FQP):
	GOBIN=$(TOOLS_DIR) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VER)
	@mv $(TOOLS_DIR)/protoc-gen-go-grpc $(PROTOC_GEN_GO_GRPC_FQP)

$(PROTOC_GEN_GO_FQP):
	GOBIN=$(TOOLS_DIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VER)
	@mv $(TOOLS_DIR)/protoc-gen-go $(PROTOC_GEN_GO_FQP)

$(BUF_FQP):
	GOBIN=$(TOOLS_DIR) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	@mv $(TOOLS_DIR)/buf $(BUF_FQP)

$(UPX_FQP):
	mkdir -p $(TOOLS_DIR)
	(cd $(TOOLS_DIR); curl -sSfLO https://github.com/upx/upx/releases/download/v$(UPX_VER)/upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux.tar.xz)
	(cd $(TOOLS_DIR); tar -xvf upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux.tar.xz)
	@mv $(TOOLS_DIR)/upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux/upx $(UPX_FQP)
	@rm -rf $(TOOLS_DIR)/upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux*

.PHONY: tools
tools: $(GOIMPORTS_FQP) $(CONTROLLER_GEN_FQP) $(PROTOC_GEN_GO_GRPC_FQP) $(PROTOC_GEN_GO_FQP) $(BUF_FQP) $(UPX_FQP) ## Install all tools