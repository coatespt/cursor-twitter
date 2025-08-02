#!/bin/bash

# tail-the-log.sh
# Automatically finds and tails the latest pipeline log file in the logs directory

# Check if logs directory exists
if [ ! -d "logs" ]; then
    echo "Error: logs directory not found in current directory"
    exit 1
fi

# Find the latest pipeline log file
latest_log=$(ls -t logs/pipeline_*.log 2>/dev/null | head -1)

if [ -z "$latest_log" ]; then
    echo "Error: No pipeline log files found in logs directory"
    echo "Expected files matching pattern: logs/pipeline_*.log"
    exit 1
fi

echo "Tailing latest pipeline log: $latest_log"
echo "Press Ctrl+C to stop"
echo "----------------------------------------"

# Tail the latest log file with follow option
tail -f "$latest_log" 