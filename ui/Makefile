# UI Makefile
# This file is included from the top-level Makefile

# Tool versions
UI_TEMPL_VER := v0.3.977
UI_BUN_VER := 1.2.5

# Detect OS and architecture for bun download
UI_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
UI_ARCH := $(shell uname -m)
ifeq ($(UI_ARCH),x86_64)
	UI_BUN_ARCH := x64
else ifeq ($(UI_ARCH),aarch64)
	UI_BUN_ARCH := aarch64
else ifeq ($(UI_ARCH),arm64)
	UI_BUN_ARCH := aarch64
endif

# Tool fully qualified paths
UI_TOOLS_DIR := $(CURDIR)/out/tools
UI_TEMPL_FQP := $(UI_TOOLS_DIR)/templ-$(UI_TEMPL_VER)
UI_BUN_FQP := $(UI_TOOLS_DIR)/bun-$(UI_BUN_VER)

# Install templ tool
$(UI_TEMPL_FQP):
	GOBIN=$(UI_TOOLS_DIR) go install github.com/a-h/templ/cmd/templ@$(UI_TEMPL_VER)
	@mv $(UI_TOOLS_DIR)/templ $(UI_TEMPL_FQP)

# Install bun tool
$(UI_BUN_FQP):
	@mkdir -p $(UI_TOOLS_DIR)
	curl -sSfL -o $(UI_TOOLS_DIR)/bun-$(UI_BUN_VER).zip https://github.com/oven-sh/bun/releases/download/bun-v$(UI_BUN_VER)/bun-$(UI_OS)-$(UI_BUN_ARCH).zip
	(cd $(UI_TOOLS_DIR) && unzip -o bun-$(UI_BUN_VER).zip)
	@mv $(UI_TOOLS_DIR)/bun-$(UI_OS)-$(UI_BUN_ARCH)/bun $(UI_BUN_FQP)
	@rm -rf $(UI_TOOLS_DIR)/bun-$(UI_OS)-$(UI_BUN_ARCH) $(UI_TOOLS_DIR)/bun-$(UI_BUN_VER).zip

.PHONY: ui-install-deps
ui-install-deps: $(UI_TEMPL_FQP) $(UI_BUN_FQP) ## Install UI dependencies (bun packages and templ)
	(cd ui && $(UI_BUN_FQP) install)

.PHONY: ui-css
ui-css: $(UI_BUN_FQP) ## Build UI CSS
	(cd ui && $(UI_BUN_FQP) run tailwindcss -i assets/css/input.css -o assets/css/output.css --minify)

.PHONY: ui-css-watch
ui-css-watch: $(UI_BUN_FQP) ## Watch and rebuild UI CSS on changes
	(cd ui && $(UI_BUN_FQP) run tailwindcss -i assets/css/input.css -o assets/css/output.css --watch)

.PHONY: ui-templ
ui-templ: $(UI_TEMPL_FQP) ## Generate templ templates
	(cd ui/templates && $(UI_TEMPL_FQP) generate)

.PHONY: ui-generate
ui-generate: ui-install-deps ui-css ui-templ ## Generate all UI files (templ templates and tailwind CSS)
	$(MAKE) fmt

.PHONY: ui-clean
ui-clean: ## Clean UI build artifacts
	rm -rf ui/node_modules/
	rm -f ui/templates/*_templ.go
	rm -f ui/assets/css/output.css

