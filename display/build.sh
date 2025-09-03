#!/bin/bash
# Build script for cursor-twitter display component

echo "Building cursor-twitter display component..."
go build -o cursor-twitter-display .

if [ $? -eq 0 ]; then
    echo "Build successful! Run with: ./cursor-twitter-display <json_file>"
else
    echo "Build failed!"
    exit 1
fi
