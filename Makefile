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

# OpenAPI generation now lives in syfon.
SYFON_DIR ?= ../syfon

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
	if [[ ! -d "$(SYFON_DIR)" ]]; then \
	  echo "ERROR: syfon repo not found at $(SYFON_DIR)"; \
	  exit 1; \
	fi; \
	echo "--> OpenAPI generation is centralized in syfon"; \
	$(MAKE) -C "$(SYFON_DIR)" gen

.PHONY: gen-internal
gen-internal:
	@set -euo pipefail; \
	if [[ ! -d "$(SYFON_DIR)" ]]; then \
	  echo "ERROR: syfon repo not found at $(SYFON_DIR)"; \
	  exit 1; \
	fi; \
	echo "--> Internal model generation is centralized in syfon"; \
	$(MAKE) -C "$(SYFON_DIR)" gen-internal

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
