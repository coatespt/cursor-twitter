#!/usr/bin/env python3
"""
Convert nested JSON format to line-by-line format for SQL loader
"""

import json
import sys

def convert_json_format(input_file, output_file):
    """Convert nested JSON to line-by-line format"""
    
    print(f"Reading {input_file}...")
    
    with open(input_file, 'r') as f:
        content = f.read()
    
    # The file appears to contain multiple JSON objects concatenated together
    # We need to split them properly
    
    batches = []
    pos = 0
    
    while pos < len(content):
        # Find the start of a JSON object
        start = content.find('{', pos)
        if start == -1:
            break
            
        # Find the matching closing brace
        brace_count = 0
        end = start
        for i in range(start, len(content)):
            if content[i] == '{':
                brace_count += 1
            elif content[i] == '}':
                brace_count -= 1
                if brace_count == 0:
                    end = i + 1
                    break
        
        if brace_count == 0:
            # We found a complete JSON object
            json_str = content[start:end]
            try:
                batch = json.loads(json_str)
                if batch.get('type') == 'batch':
                    batches.append(batch)
                    print(f"Found batch {batch['data']['batch_number']}")
            except json.JSONDecodeError as e:
                print(f"Failed to parse JSON at position {start}: {e}")
                # Skip this malformed JSON
                
            pos = end
        else:
            # Incomplete JSON object, break
            break
    
    print(f"Found {len(batches)} batches")
    
    # Write each batch as a separate line
    print(f"Writing to {output_file}...")
    with open(output_file, 'w') as f:
        for batch in batches:
            f.write(json.dumps(batch) + '\n')
    
    print(f"Conversion complete! {len(batches)} batches written to {output_file}")

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python convert_json_format.py <input.json> <output.json>")
        sys.exit(1)
    
    input_file = sys.argv[1]
    output_file = sys.argv[2]
    
    convert_json_format(input_file, output_file)
