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
	@echo "  build  - Build the process executable"
	@echo "  clean  - Remove build artifacts"
	@echo "  test   - Run tests"
	@echo "  run    - Run the program with sample data"
	@echo "  help   - Show this help message"

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