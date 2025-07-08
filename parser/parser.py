#!/usr/bin/env python3

import gzip
import json
import csv
import sys
import os
import glob
from pathlib import Path
import time

def process_json_file(input_path, output_path, global_tweet_count):
    """Process a single gzipped JSON file and convert to CSV."""
    print(f"Processing {input_path} -> {output_path}")
    
    # Remove existing output file if it exists
    if os.path.exists(output_path):
        os.remove(output_path)
    
    tweet_count = 0
    non_blank_lines = 0
    bracket_lines = 0
    comma_lines = 0
    delete_events = 0
    scrub_geo_events = 0
    json_decode_errors = 0
    missing_fields = 0
    start_time = time.time()
    
    # Try different encodings
    encodings = ['utf-8', 'latin-1', 'cp1252', 'iso-8859-1']
    
    for encoding in encodings:
        try:
            with gzip.open(input_path, 'rt', encoding=encoding, errors='replace') as f_in, \
                 open(output_path, 'w', newline='', encoding='utf-8') as f_out:
                
                writer = csv.writer(f_out)
                
                # Write CSV header
                writer.writerow([
                    'id_str', 'created_at', 'user_id_str', 'retweet_count', 
                    'text', 'retweeted', 'at', 'http', 'hashtag', 'words'
                ])
                
                for line_num, line in enumerate(f_in, 1):
                    line = line.strip()
                    if not line:
                        continue
                    non_blank_lines += 1
                    
                    # Skip opening and closing brackets
                    if line == '[' or line == ']':
                        bracket_lines += 1
                        continue
                    
                    # Skip lines that are just a comma
                    if line == ',':
                        comma_lines += 1
                        continue
                    
                    # Remove trailing comma (fix: use rstrip for string)
                    line = line.rstrip(',')
                    
                    try:
                        # Parse JSON object
                        tweet = json.loads(line)
                        
                        # Skip delete and scrub_geo events
                        if 'delete' in tweet:
                            delete_events += 1
                            continue
                        if 'scrub_geo' in tweet:
                            scrub_geo_events += 1
                            continue
                        
                        # Only check for presence of 'id_str', 'text', and 'user'
                        if 'id_str' in tweet and 'text' in tweet and 'user' in tweet:
                            id_str = tweet.get('id_str', '')
                            created_at = tweet.get('created_at', '')
                            user_id_str = tweet.get('user', {}).get('id_str', '')
                            retweet_count = tweet.get('retweet_count', 0)
                            text = tweet.get('text', '')
                            retweeted = tweet.get('retweeted', False)
                            # Count patterns
                            at_count = text.count('@')
                            http_count = text.count('http')
                            hashtag_count = text.count('#')
                            # Extract words (simple approach)
                            words = ' '.join([
                                word.lower() for word in text.split()
                                if word.isalpha() and len(word) > 1
                            ])
                            writer.writerow([
                                id_str, created_at, user_id_str, retweet_count,
                                text, retweeted, at_count, http_count, hashtag_count, words
                            ])
                            tweet_count += 1
                            if tweet_count % 1000 == 0:
                                elapsed = time.time() - start_time
                                tps = tweet_count / elapsed if elapsed > 0 else 0
                                print(f"Processed {tweet_count} tweets in this file (total: {global_tweet_count + tweet_count}) [{tps:.0f} tweets/sec]", end='\r', flush=True)
                        else:
                            missing_fields += 1
                            print(f"Line {line_num}: missing one of id_str, text, or user")
                            
                    except json.JSONDecodeError as e:
                        json_decode_errors += 1
                        print(f"Failed to decode JSON at line {line_num}: {e}")
                        continue
                    except Exception as e:
                        print(f"Error processing line {line_num}: {e}")
                        continue
            
            # If we get here, the encoding worked
            print(f"Successfully processed with {encoding} encoding")
            break
            
        except UnicodeDecodeError:
            print(f"Failed with {encoding} encoding, trying next...")
            continue
        except Exception as e:
            print(f"Failed to process {input_path} with {encoding} encoding: {e}")
            continue
    else:
        print(f"Failed to process {input_path} with any encoding")
        return 0, 0
    
    print(f"File summary for {input_path}:")
    print(f"  Non-blank lines read: {non_blank_lines}")
    print(f"  CSV rows created: {tweet_count}")
    print(f"  Bracket lines skipped: {bracket_lines}")
    print(f"  Comma lines skipped: {comma_lines}")
    print(f"  Delete events skipped: {delete_events}")
    print(f"  Scrub_geo events skipped: {scrub_geo_events}")
    print(f"  JSON decode errors: {json_decode_errors}")
    print(f"  Missing required fields: {missing_fields}")
    print()
    return tweet_count, (bracket_lines + comma_lines + delete_events + scrub_geo_events + json_decode_errors + missing_fields)

def main():
    if len(sys.argv) != 3:
        print("Usage: python parser.py <input_dir> <output_dir>")
        sys.exit(1)
    
    input_dir = sys.argv[1]
    output_dir = sys.argv[2]
    
    # Create output directory if it doesn't exist
    os.makedirs(output_dir, exist_ok=True)
    
    # Find all .json.gz files
    pattern = os.path.join(input_dir, "*.json.gz")
    files = glob.glob(pattern)
    
    if not files:
        print(f"No .json.gz files found in {input_dir}")
        sys.exit(1)
    
    total_tweets = 0
    total_skipped = 0
    
    for gz_file in files:
        base_name = os.path.basename(gz_file)
        csv_file = os.path.join(output_dir, base_name.replace('.json.gz', '.csv'))
        
        try:
            tweets, skipped = process_json_file(gz_file, csv_file, total_tweets)
            total_tweets += tweets
            total_skipped += skipped
        except Exception as e:
            print(f"Failed to process {gz_file}: {e}")
    
    print(f"Total: {total_tweets} tweets, {total_skipped} skipped")

if __name__ == "__main__":
    main() 