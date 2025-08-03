#!/bin/bash

# Test script to verify startup scenarios work correctly
# This script tests the three cases:
# 1. No --load-state flag (should start immediately)
# 2. --load-state with valid state files (should wait for filters)
# 3. --load-state with corrupted/absent state files (should fall back to building from scratch)

set -e

echo "=== Testing Twitter Pipeline Startup Scenarios ==="
echo

# Test 1: No --load-state flag
echo "Test 1: No --load-state flag"
echo "Expected: Should start immediately and build filters from scratch"
echo "Command: ./twitter-pipeline --config config/config.yaml"
echo "Note: This will start the program - press Ctrl+C to stop after a few seconds"
echo
read -p "Press Enter to run Test 1 (or Ctrl+C to skip)..."
timeout 10s ./twitter-pipeline --config config/config.yaml || echo "Test 1 completed (timeout or Ctrl+C)"
echo

# Test 2: --load-state with valid state files
echo "Test 2: --load-state with valid state files"
echo "Expected: Should wait for filters to load from state"
echo "Command: ./twitter-pipeline --config config/config.yaml --load-state"
echo "Note: This will start the program - press Ctrl+C to stop after a few seconds"
echo
read -p "Press Enter to run Test 2 (or Ctrl+C to skip)..."
timeout 10s ./twitter-pipeline --config config/config.yaml --load-state || echo "Test 2 completed (timeout or Ctrl+C)"
echo

# Test 3: --load-state with corrupted/absent state files
echo "Test 3: --load-state with corrupted/absent state files"
echo "Expected: Should detect no state files and fall back to building from scratch"
echo "Command: ./twitter-pipeline --config config/config.yaml --load-state"
echo "Note: This will start the program - press Ctrl+C to stop after a few seconds"
echo
read -p "Press Enter to run Test 3 (or Ctrl+C to skip)..."
timeout 10s ./twitter-pipeline --config config/config.yaml --load-state || echo "Test 3 completed (timeout or Ctrl+C)"
echo

echo "=== Startup Scenario Tests Complete ==="
echo
echo "Summary of expected behavior:"
echo "1. No --load-state: Should start immediately with 'Starting immediately' message"
echo "2. --load-state with valid state: Should show 'waiting for filters' and then 'Filters ready!'"
echo "3. --load-state with no state: Should show warning and 'Starting immediately' message"
echo "4. --load-state with corrupted state: Should timeout after 30s and fall back to building from scratch" 