#!/bin/bash

# Test Runner Script for Twitter Data Processing Pipeline
# This script runs all tests in the project and provides clear output

set -e  # Exit on any error

echo "=========================================="
echo "Twitter Data Processing Pipeline - Test Suite"
echo "=========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    case $status in
        "PASS")
            echo -e "${GREEN}✓ PASS${NC}: $message"
            ;;
        "FAIL")
            echo -e "${RED}✗ FAIL${NC}: $message"
            ;;
        "SKIP")
            echo -e "${YELLOW}⚠ SKIP${NC}: $message"
            ;;
        "INFO")
            echo -e "${BLUE}ℹ INFO${NC}: $message"
            ;;
    esac
}

# Function to run tests for a specific package
run_package_tests() {
    local package=$1
    local package_name=$2
    
    echo -e "\n${BLUE}Testing $package_name...${NC}"
    
    if [ -d "$package" ]; then
        if go test -v ./$package/... 2>&1; then
            print_status "PASS" "$package_name tests completed successfully"
        else
            print_status "FAIL" "$package_name tests failed"
            return 1
        fi
    else
        print_status "SKIP" "$package_name directory not found"
    fi
}

# Function to run specific test files
run_test_files() {
    local test_dir=$1
    local test_name=$2
    
    echo -e "\n${BLUE}Running $test_name...${NC}"
    
    if [ -d "$test_dir" ]; then
        if go test -v ./$test_dir/... 2>&1; then
            print_status "PASS" "$test_name completed successfully"
        else
            print_status "FAIL" "$test_name failed"
            return 1
        fi
    else
        print_status "SKIP" "$test_name directory not found"
    fi
}

# Start time
start_time=$(date +%s)

print_status "INFO" "Starting test suite at $(date)"

# Check if we're in the right directory
if [ ! -f "go.mod" ]; then
    print_status "FAIL" "go.mod not found. Please run this script from the project root directory."
    exit 1
fi

print_status "INFO" "Project root directory confirmed"

# Check Go installation
if ! command -v go &> /dev/null; then
    print_status "FAIL" "Go is not installed or not in PATH"
    exit 1
fi

print_status "INFO" "Go version: $(go version)"

# Clean and download dependencies
echo -e "\n${BLUE}Preparing dependencies...${NC}"
go mod tidy
go mod download
print_status "PASS" "Dependencies prepared"

# Run all tests with coverage
echo -e "\n${BLUE}Running all tests with coverage...${NC}"
if go test -v -coverprofile=coverage.out ./... 2>&1; then
    print_status "PASS" "All tests completed successfully"
else
    print_status "FAIL" "Some tests failed"
    exit 1
fi

# Generate coverage report
if [ -f "coverage.out" ]; then
    echo -e "\n${BLUE}Generating coverage report...${NC}"
    go tool cover -func=coverage.out
    echo ""
    go tool cover -html=coverage.out -o coverage.html
    print_status "PASS" "Coverage report generated (coverage.html)"
fi

# Run specific package tests for detailed output
run_package_tests "src" "Source Code"
run_package_tests "src/filter" "Word Filter"
run_package_tests "src/pipeline" "Pipeline Components"
run_package_tests "src/tweets" "Tweet Processing"
run_package_tests "tests" "Integration Tests"

# Run benchmarks if they exist
echo -e "\n${BLUE}Running benchmarks...${NC}"
if go test -bench=. ./... 2>&1; then
    print_status "PASS" "Benchmarks completed"
else
    print_status "SKIP" "No benchmarks found or benchmarks failed"
fi

# Check for race conditions
echo -e "\n${BLUE}Checking for race conditions...${NC}"
if go test -race ./... 2>&1; then
    print_status "PASS" "No race conditions detected"
else
    print_status "FAIL" "Race conditions detected"
    exit 1
fi

# Run Python status tracking test
echo -e "\n${BLUE}Running Python status tracking test...${NC}"
if python3 sender/test_status_tracking.py; then
    print_status "PASS" "Python status tracking test passed"
else
    print_status "FAIL" "Python status tracking test failed"
    exit 1
fi

# End time and duration
end_time=$(date +%s)
duration=$((end_time - start_time))

echo -e "\n=========================================="
echo -e "${GREEN}Test Suite Completed Successfully!${NC}"
echo -e "Duration: ${duration} seconds"
echo -e "Completed at: $(date)"
echo -e "=========================================="

# Clean up
rm -f coverage.out

print_status "INFO" "Test artifacts cleaned up"
print_status "INFO" "Coverage report saved as coverage.html" 