# --- Variables ---
# The name of the resulting binary (e.g., 'data-client' if your module is called data-client)
# Update this if your main package is not in the root directory.
TARGET_NAME := $(shell basename $(shell pwd))

# The default path to build the main package. Use '.' if your main package is in the root.
# Change this if your main package is in a subdirectory (e.g., ./cmd/myapp)
MAIN_PACKAGE := .

# The directory where the final binary will be placed
BIN_DIR := ./bin

# Coverage thresholds
COVERAGE_THRESHOLD := 30
PACKAGE_COVERAGE_THRESHOLD := 20

# OpenAPI Generation Variables
OPENAPI ?= ga4gh/data-repository-service-schemas/openapi/data_repository_service.openapi.yaml
OAG_IMAGE ?= openapitools/openapi-generator-cli:latest
REDOCLY_IMAGE ?= redocly/cli:latest
YQ_IMAGE ?= mikefarah/yq:latest
GEN_OUT ?= .tmp/apigen.gen
INTERNAL_OPENAPI ?= apigen/api/internal.openapi.yaml
INTERNAL_GEN_OUT ?= .tmp/apigen-internal.gen
SCHEMAS_SUBMODULE ?= ga4gh/data-repository-service-schemas

# --- Targets ---

.PHONY: all build test test-coverage coverage-html coverage-check generate tidy clean help

# The default target run when you type 'make'
all: build

## build: Compiles the application binary
build:
	@echo "--> Building $(TARGET_NAME)..."
	@go build -o $(BIN_DIR)/$(TARGET_NAME) $(MAIN_PACKAGE)
	@echo "Build successful! Binary placed in $(BIN_DIR)/$(TARGET_NAME)"

## test: Runs all unit tests (including tests in subdirectories)
test:
	@echo "--> Running all tests..."
	@go test -v ./...

## test-coverage: Runs tests with coverage profiling
test-coverage:
	@echo "--> Running tests with coverage..."
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@echo "--> Coverage report generated: coverage.out"
	@go tool cover -func=coverage.out | tail -1

## coverage-html: Generates HTML coverage report
coverage-html: test-coverage
	@echo "--> Generating HTML coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "--> HTML coverage report generated: coverage.html"

## coverage-check: Verifies coverage meets minimum thresholds
coverage-check: test-coverage
	@echo "--> Checking coverage thresholds..."
	@./scripts/check-coverage.sh $(COVERAGE_THRESHOLD) $(PACKAGE_COVERAGE_THRESHOLD)

## generate: Runs go generate commands to create mocks, embedded assets, etc.
generate:
	@echo "--> Running code generation (go generate)..."
	@go generate ./...

## gen: Generates Go models from OpenAPI specs
gen:
	@set -euo pipefail; \
	mkdir -p .tmp; \
	spec="$(OPENAPI)"; \
	if [[ ! -f "$$spec" ]]; then \
	  echo "ERROR: OpenAPI spec '$$spec' not found. Run: make init-schemas"; \
	  exit 1; \
	fi; \
	if ! command -v docker >/dev/null 2>&1; then \
	  echo "ERROR: docker is required for 'make gen'."; \
	  exit 1; \
	fi; \
	echo "Bundling canonical OpenAPI spec with Redocly..."; \
	docker run --rm \
	  --user "$$(id -u):$$(id -g)" \
	  -v "$(PWD):/local" \
	  $(REDOCLY_IMAGE) bundle /local/$$spec --output /local/.tmp/drs.base.yaml --ext yaml; \
	echo "Merging internal Extensions with yq..."; \
	docker run --rm \
	  --user "$$(id -u):$$(id -g)" \
	  -v "$(PWD):/local" \
	  $(YQ_IMAGE) eval-all 'select(fileIndex == 0) * select(fileIndex == 1)' /local/.tmp/drs.base.yaml /local/apigen/specs/drs-extensions-overlay.yaml > apigen/api/openapi.yaml; \
	rm -rf "$(GEN_OUT)"; \
	docker run --rm --pull=missing \
	  --user "$$(id -u):$$(id -g)" \
	  -v "$(PWD):/local" \
	  $(OAG_IMAGE) generate \
	  -g go \
	  --skip-validate-spec \
	  --git-repo-id data-client \
	  --git-user-id calypr \
	  -i /local/apigen/api/openapi.yaml \
	  -o /local/$(GEN_OUT) \
	  --global-property models,modelDocs=false,modelTests=false,supportingFiles=utils.go \
	  --additional-properties packageName=drs,enumClassPrefix=true; \
	mkdir -p apigen/api apigen; \
	rm -rf apigen/drs; \
	mkdir -p apigen/drs; \
	find "$(GEN_OUT)" -maxdepth 1 -type f -name '*.go' -exec mv {} apigen/drs/ \; ; \
	echo "Generated DRS client models into ./apigen/drs"; \
	if [[ -f "$(INTERNAL_OPENAPI)" ]]; then $(MAKE) gen-internal; fi

.PHONY: gen-internal
gen-internal:
	@set -euo pipefail; \
	rm -rf "$(INTERNAL_GEN_OUT)"; \
	docker run --rm --pull=missing \
	  --user "$$(id -u):$$(id -g)" \
	  -v "$(PWD):/local" \
	  $(OAG_IMAGE) generate \
	  -g go \
	  --skip-validate-spec \
	  --git-repo-id data-client \
	  --git-user-id calypr \
	  -i /local/apigen/api/internal.openapi.yaml \
	  -o /local/$(INTERNAL_GEN_OUT) \
	  --global-property models,modelDocs=false,modelTests=false,supportingFiles=utils.go \
	  --additional-properties packageName=internalapi,enumClassPrefix=true; \
	rm -rf apigen/internalapi; \
	mkdir -p apigen/internalapi; \
	find "$(INTERNAL_GEN_OUT)" -maxdepth 1 -type f -name '*.go' -exec mv {} apigen/internalapi/ \; ; \
	echo "Generated Internal models into ./apigen/internalapi"

.PHONY: init-schemas
init-schemas:
	@git submodule update --init --recursive --depth 1 "$(SCHEMAS_SUBMODULE)"

## tidy: Cleans up module dependencies and formats go files
tidy:
	@echo "--> Tidying go.mod and formatting files..."
	@go mod tidy
	@go fmt ./...

## clean: Removes the compiled binary and coverage files
clean:
	@echo "--> Cleaning up..."
	@rm -f $(BIN_DIR)/$(TARGET_NAME)
	@rm -f coverage.out coverage.html
	@rm -rf .tmp
