# --- Variables ---
# The name of the resulting binary (e.g., 'data-client' if your module is called data-client)
# Update this if your main package is not in the root directory.
TARGET_NAME := $(shell basename $(shell pwd))

# The default path to build the main package. Use '.' if your main package is in the root.
# Change this if your main package is in a subdirectory (e.g., ./cmd/myapp)
MAIN_PACKAGE := .

# The directory where the final binary will be placed
BIN_DIR := ./bin

# --- Targets ---

.PHONY: all build test generate tidy clean help

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

## generate: Runs go generate commands to create mocks, embedded assets, etc.
generate:
	@echo "--> Running code generation (go generate)..."
	@go generate ./...

## tidy: Cleans up module dependencies and formats go files
tidy:
	@echo "--> Tidying go.mod and formatting files..."
	@go mod tidy
	@go fmt ./...

## clean: Removes the compiled binary
clean:
	@echo "--> Cleaning up..."
	@rm -f $(BIN_DIR)/$(TARGET_NAME)

