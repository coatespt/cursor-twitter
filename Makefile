.PHONY: build clean test run help

# Default target
all: build

# Build the executable
build:
	@echo "Building process executable..."
	go build -o process src/process.go
	@echo "Build complete!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f process
	@echo "Clean complete!"

# Run tests (if any)
test:
	@echo "Running tests..."
	go test ./...
	@echo "Tests complete!"

# Run the program with sample data
run:
	@echo "Running process with sample data..."
	./process ../twits/msg_input ../twits/msg_output --auto-clean

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the process executable"
	@echo "  clean        - Remove build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-all     - Run all tests (same as test)"
	@echo "  test-verbose - Run tests with verbose output"
	@echo "  test-coverage- Run tests with coverage report"
	@echo "  test-race    - Run tests with race condition detection"
	@echo "  test-bench   - Run benchmarks"
	@echo "  test-full    - Run all tests with full analysis"
	@echo "  run          - Run the program with sample data"
	@echo "  deps         - Install dependencies"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  help         - Show this help message"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy
	@echo "Dependencies installed!"

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "Code formatted!"

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run
	@echo "Linting complete!" 

test-all:
	go test ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	go test -v ./...
	@echo "Verbose tests complete!"

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo "Coverage report complete!"

# Run tests with race condition detection
test-race:
	@echo "Running tests with race condition detection..."
	go test -race ./...
	@echo "Race condition tests complete!"

# Run benchmarks
test-bench:
	@echo "Running benchmarks..."
	go test -bench=. ./...
	@echo "Benchmarks complete!"

# Run all tests with full analysis
test-full: test-verbose test-coverage test-race test-bench
	@echo "Full test suite complete!" 