.PHONY: build clean test run help

# Default target
all: build

# Build the executable
build:
	@echo "Building twitter-pipeline executable..."
	go build -o twitter-pipeline src/*.go
	@echo "Build complete!"

# Build language detector
build-language-detector:
	@echo "Building language detector..."
	go build -o language_detector util_go/language_detector.go
	@echo "Language detector build complete!"

# Build CSV file finder
build-find-csv:
	@echo "Building CSV file finder..."
	go build -o find_csv_file util_go/find_csv_file.go
	@echo "CSV file finder build complete!"

# Build CSV file mapping
build-csv-mapping:
	@echo "Building CSV file mapping..."
	go build -o csv_file_mapping util_go/csv_file_mapping.go
	@echo "CSV file mapping build complete!"

# Build token frequency analyzer
build-token-frequency:
	@echo "Building token frequency analyzer..."
	go build -o token_frequency_analyzer util_go/token_frequency_analyzer.go
	@echo "Token frequency analyzer build complete!"

# Build token analyzer
build-analyze-tokens:
	@echo "Building token analyzer..."
	go build -o analyze_tokens util_go/analyze_tokens.go
	@echo "Token analyzer build complete!"

# Build token examiner
build-examine-tokens:
	@echo "Building token examiner..."
	go build -o examine_tokens util_go/examine_tokens.go
	@echo "Token examiner build complete!"

# Build display component
build-display:
	@echo "Building display component..."
	cd display && ./build.sh
	@echo "Display component build complete!"

# Build all components (main + display)
build-all: build build-display
	@echo "All components built!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f twitter-pipeline language_detector find_csv_file csv_file_mapping token_frequency_analyzer analyze_tokens examine_tokens
	rm -f display/cursor-twitter-display
	@echo "Clean complete!"

# Run tests (if any)
test:
	@echo "Running tests..."
	go test ./...
	@echo "Tests complete!"

# Run the program with sample data
run:
	@echo "Running twitter-pipeline with sample data..."
	./twitter-pipeline

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the process executable"
	@echo "  build-language-detector - Build the language detector"
	@echo "  build-find-csv        - Build the CSV file finder"
	@echo "  build-csv-mapping     - Build the CSV file mapping"
	@echo "  build-token-frequency - Build the token frequency analyzer"
	@echo "  build-analyze-tokens  - Build the token analyzer"
	@echo "  build-examine-tokens  - Build the token examiner"
	@echo "  build-display         - Build the display component"
	@echo "  build-all             - Build all components (main + display)"
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