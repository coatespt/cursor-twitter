#!/bin/bash

# JSON Output Analysis Script
# Analyzes Twitter pipeline JSON output files and provides comprehensive statistics

if [ $# -eq 0 ]; then
    echo "Usage: $0 <json_file>"
    echo "Example: $0 september_18.json"
    exit 1
fi

JSON_FILE="$1"

if [ ! -f "$JSON_FILE" ]; then
    echo "Error: File '$JSON_FILE' not found"
    exit 1
fi

echo "=== JSON Output Analysis for: $JSON_FILE ==="
echo

# File size
FILE_SIZE=$(du -h "$JSON_FILE" | cut -f1)
echo "📁 File size: $FILE_SIZE"
echo

# Count batches (with error handling) - handle both JSON array and JSONL formats
BATCH_COUNT=$(jq -r 'select(type == "object" and .type == "batch")' "$JSON_FILE" 2>/dev/null | wc -l)
if [ $? -ne 0 ] || [ -z "$BATCH_COUNT" ]; then
    echo "❌ Error parsing JSON file. The file may contain malformed JSON entries."
    echo "   This can happen if log messages or other text got mixed into the JSON output."
    echo "   Try running the pipeline again or check the file for non-JSON content."
    exit 1
fi

echo "📊 Total batches: $BATCH_COUNT"
echo

if [ "$BATCH_COUNT" -eq 0 ]; then
    echo "❌ No batches found in the file"
    exit 1
fi

# Extract cluster counts per batch (with error handling) - handle JSONL format
echo "🔍 Analyzing cluster statistics..."
CLUSTER_COUNTS=$(jq -r 'select(type == "object" and .type == "batch") | .data.clusters | length' "$JSON_FILE" 2>/dev/null)
if [ $? -ne 0 ] || [ -z "$CLUSTER_COUNTS" ]; then
    echo "❌ Error extracting cluster data. Some entries may be malformed."
    exit 1
fi

# Calculate cluster statistics
CLUSTER_MIN=$(echo "$CLUSTER_COUNTS" | jq 'min')
CLUSTER_MAX=$(echo "$CLUSTER_COUNTS" | jq 'max')
CLUSTER_AVG=$(echo "$CLUSTER_COUNTS" | jq 'add / length')
CLUSTER_STDEV=$(echo "$CLUSTER_COUNTS" | jq -r '
    . as $data |
    ($data | add / length) as $mean |
    ($data | map((. - $mean) * (. - $mean)) | add / length | sqrt)
')

echo "📈 Cluster statistics per batch:"
echo "   Min clusters: $CLUSTER_MIN"
echo "   Max clusters: $CLUSTER_MAX"
echo "   Average clusters: $(printf "%.2f" $CLUSTER_AVG)"
echo "   Standard deviation: $(printf "%.2f" $CLUSTER_STDEV)"
echo

# Extract tweet counts per batch (with error handling) - handle JSONL format
echo "🐦 Analyzing tweet statistics..."
TWEET_COUNTS=$(jq -r 'select(type == "object" and .type == "batch") | .data.total_tweets' "$JSON_FILE" 2>/dev/null)
if [ $? -ne 0 ] || [ -z "$TWEET_COUNTS" ]; then
    echo "❌ Error extracting tweet data. Some entries may be malformed."
    exit 1
fi

# Calculate tweet statistics
TWEET_MIN=$(echo "$TWEET_COUNTS" | jq 'min')
TWEET_MAX=$(echo "$TWEET_COUNTS" | jq 'max')
TWEET_AVG=$(echo "$TWEET_COUNTS" | jq 'add / length')
TWEET_STDEV=$(echo "$TWEET_COUNTS" | jq -r '
    . as $data |
    ($data | add / length) as $mean |
    ($data | map((. - $mean) * (. - $mean)) | add / length | sqrt)
')

echo "📈 Tweet statistics per batch:"
echo "   Min tweets: $TWEET_MIN"
echo "   Max tweets: $TWEET_MAX"
echo "   Average tweets: $(printf "%.0f" $TWEET_AVG)"
echo "   Standard deviation: $(printf "%.0f" $TWEET_STDEV)"
echo

# Additional statistics
TOTAL_TWEETS=$(echo "$TWEET_COUNTS" | jq 'add')
TOTAL_CLUSTERS=$(echo "$CLUSTER_COUNTS" | jq 'add')

echo "📊 Summary statistics:"
echo "   Total tweets processed: $TOTAL_TWEETS"
echo "   Total clusters created: $TOTAL_CLUSTERS"
echo "   Average clusters per tweet: $(echo "scale=4; $TOTAL_CLUSTERS / $TOTAL_TWEETS" | bc -l)"
echo

# Batch time range - handle JSONL format
echo "⏰ Time analysis..."
FIRST_BATCH_TIME=$(jq -r 'select(type == "object" and .type == "batch") | .data.batch_time' "$JSON_FILE" 2>/dev/null | head -1)
LAST_BATCH_TIME=$(jq -r 'select(type == "object" and .type == "batch") | .data.batch_time' "$JSON_FILE" 2>/dev/null | tail -1)

echo "   First batch: $FIRST_BATCH_TIME"
echo "   Last batch: $LAST_BATCH_TIME"
echo

# Clusters above minimum size - handle JSONL format
echo "📏 Clusters above minimum size analysis..."
CLUSTERS_ABOVE_MIN=$(jq -r 'select(type == "object" and .type == "batch") | .data.clusters_above_min_size' "$JSON_FILE" 2>/dev/null)
ABOVE_MIN_AVG=$(echo "$CLUSTERS_ABOVE_MIN" | jq 'add / length')
ABOVE_MIN_TOTAL=$(echo "$CLUSTERS_ABOVE_MIN" | jq 'add')

echo "   Average clusters above min size per batch: $(printf "%.2f" $ABOVE_MIN_AVG)"
echo "   Total clusters above min size: $ABOVE_MIN_TOTAL"
echo

# File structure validation - handle JSONL format
echo "🔍 File structure validation..."
BATCH_TYPES=$(jq -r 'select(type == "object") | .type' "$JSON_FILE" 2>/dev/null | sort | uniq)
echo "   Data types found: $BATCH_TYPES"

# Check for any errors or warnings - handle JSONL format
ERROR_COUNT=$(jq -r 'select(type == "object" and .type == "error")' "$JSON_FILE" 2>/dev/null | wc -l)
if [ "$ERROR_COUNT" -gt 0 ]; then
    echo "   ⚠️  Found $ERROR_COUNT error entries"
else
    echo "   ✅ No error entries found"
fi

echo
echo "=== Analysis Complete ==="
