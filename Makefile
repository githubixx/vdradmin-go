.PHONY: build run test clean lint docker help

BINARY_NAME=vdradmin
BUILD_DIR=build
GO_FILES=$(shell find . -name '*.go' -type f)

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## build: Build the application
build:
	@echo "Building..."
	@go build -o ${BUILD_DIR}/${BINARY_NAME} ./cmd/vdradmin

## run: Run the application
run:
	@echo "Running..."
	@go run ./cmd/vdradmin

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
