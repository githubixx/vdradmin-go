.PHONY: arch-build build build-mcp build-web clean deps docker fmt help install lint run run-mcp test test-coverage

BINARY_NAME ?= vdradmin-go
MCP_BINARY_NAME ?= vdradmin-go-mcp
BUILD_DIR ?= build
CMD_DIR ?= ./cmd/vdradmin-go
MCP_CMD_DIR ?= ./cmd/vdradmin-go-mcp
VERSION ?= dev
COMMIT ?= none
DATE ?= unknown
LDFLAGS ?= -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
PREFIX ?= /usr
BINDIR ?= $(PREFIX)/bin
DATADIR ?= $(PREFIX)/share/vdradmin-go
DESTDIR ?=

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## build: Build the application
build: build-web build-mcp

## build-web: Build the web application
build-web:
	@echo "Building web application..."
	@mkdir -p ${BUILD_DIR}
	@go build -mod=readonly -trimpath -buildvcs=false -ldflags "${LDFLAGS}" -o ${BUILD_DIR}/${BINARY_NAME} ${CMD_DIR}

## arch-build: Build Arch package-ready binaries
arch-build: build

## build-mcp: Build the MCP server
build-mcp:
	@echo "Building MCP server..."
	@mkdir -p ${BUILD_DIR}
	@go build -mod=readonly -trimpath -buildvcs=false -ldflags "${LDFLAGS}" -o ${BUILD_DIR}/${MCP_BINARY_NAME} ${MCP_CMD_DIR}

## run: Run the application
run:
	@echo "Running..."
	@go run ${CMD_DIR}

## install: Install the binary and web assets (honors DESTDIR)
install: build
	@install -Dm755 ${BUILD_DIR}/${BINARY_NAME} ${DESTDIR}${BINDIR}/${BINARY_NAME}
	@install -Dm755 ${BUILD_DIR}/${MCP_BINARY_NAME} ${DESTDIR}${BINDIR}/${MCP_BINARY_NAME}
	@mkdir -p ${DESTDIR}${DATADIR}
	@cp -a web ${DESTDIR}${DATADIR}/

## run-mcp: Run the MCP server over stdio
run-mcp:
	@echo "Running MCP server over stdio..."
	@go run ${MCP_CMD_DIR}

## test: Run tests
test:
	@echo "Running tests..."
	@go test -v -race -cover ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run linters
lint:
	@echo "Running linters..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@which gofumpt > /dev/null && gofumpt -w . || true

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf ${BUILD_DIR}
	@rm -f coverage.out coverage.html

## docker: Build Docker image
docker:
	@echo "Building Docker image..."
	@docker build -t vdradmin-go:latest -f deployments/Dockerfile .

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

.DEFAULT_GOAL := help
