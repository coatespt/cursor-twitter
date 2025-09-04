#!/bin/bash

# Usage: ./extract_geo_location.sh input_file.json
# Extracts non-null geo and user location fields from Twitter JSON data

input_file="${1}"

if [ -z "$input_file" ] || [ ! -f "$input_file" ]; then
    echo "Usage: $0 <json_file>"
    echo "Error: File '$input_file' not found or not specified"
    exit 1
fi

echo "Checking file format..."
head -n 1 "$input_file"
echo "---"

echo "=== Lines with non-null geo data ==="
grep -n '"geo":[^n]' "$input_file" | head -5

echo -e "\n=== Lines with non-empty location ==="
grep -n '"location":"[^"]' "$input_file" | head -5

echo -e "\n=== Raw search for geo coordinates ==="
grep -o '"geo":{[^}]*}' "$input_file" | head -5

echo -e "\n=== Raw search for locations ==="
grep -o '"location":"[^"]*"' "$input_file" | grep -v '"location":""' | head -10
