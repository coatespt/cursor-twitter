#!/usr/bin/env python3

import gzip
import json
import csv
import sys
import os
import glob
from pathlib import Path
import time
import tempfile
import shutil
# Requires: langid (pip install langid)
import langid
import yaml  # For reading config
import argparse
import multiprocessing

# Load config.yaml to check if language detection should be enabled
with open(os.path.join(os.path.dirname(__file__), '../config/config.yaml')) as f:
    config = yaml.safe_load(f)
detect_language = config.get('detect_language', False)
drop_excessive_questions = config.get('drop_excessive_questions', False)

def is_too_many_questions(text, threshold=0.2, min_count=10):
    if not text:
        return False
    q_count = text.count('?')
    if q_count >= min_count and (q_count / len(text)) > threshold:
        return True
    return False

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

    # Use a temporary file in /tmp for extraction
    with tempfile.TemporaryDirectory(prefix="parser_tmp_") as temp_dir:
        temp_gz_path = os.path.join(temp_dir, os.path.basename(input_path))
        shutil.copy2(input_path, temp_gz_path)
        for encoding in encodings:
            try:
                with gzip.open(temp_gz_path, 'rt', encoding=encoding, errors='replace') as f_in, \
                     open(output_path, 'w', newline='', encoding='utf-8') as f_out:
                    writer = csv.writer(f_out, lineterminator='\n')
                    # Write CSV header
                    writer.writerow([
                        'id_str', 'created_at', 'user_id_str', 'retweet_count', 
                        'text', 'retweeted', 'at', 'http', 'hashtag', 'words', 'lang'
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
                                # Clean up the text to prevent CSV parsing issues
                                text = text.replace('\n', ' ').replace('\r', ' ')
                                text = text.replace('"', "'")
                                text = ''.join(char for char in text if ord(char) >= 32 or char in '\t\n\r')
                                # Drop tweets with excessive question marks if enabled
                                if drop_excessive_questions and is_too_many_questions(text):
                                    continue
                                at_count = text.count('@')
                                http_count = text.count('http')
                                hashtag_count = text.count('#')
                                words = ' '.join([
                                    word.lower() for word in text.split()
                                    if word.isalpha() and len(word) > 1
                                ])
                                # Detect language of tweet text if enabled in config and not disabled by command line
                                if detect_language and not args.no_language_detect:
                                    tweet_text = tweet.get('text', '')
                                    if tweet_text.strip():
                                        detected_lang, _ = langid.classify(tweet_text)
                                    else:
                                        detected_lang = 'xx'
                                    lang_out = detected_lang
                                else:
                                    # Use original lang field (may be inaccurate)
                                    lang_out = tweet.get('user', {}).get('lang', '')
                                writer.writerow([
                                    id_str, created_at, user_id_str, retweet_count,
                                    text, retweeted, at_count, http_count, hashtag_count, words, lang_out
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

def process_json_file_worker(args):
    gz_file, csv_file = args
    try:
        tweets, skipped = process_json_file(gz_file, csv_file, 0)
        return (gz_file, tweets, skipped, None)
    except Exception as e:
        return (gz_file, 0, 0, str(e))


def main():
    parser = argparse.ArgumentParser(description="Parse gzipped JSON tweet files to CSV.")
    parser.add_argument("input_dir", help="Input directory containing .json.gz files")
    parser.add_argument("output_dir", help="Output directory for .csv files")
    parser.add_argument("--num-workers", type=int, default=multiprocessing.cpu_count(),
                        help="Number of worker processes (default: number of CPU cores)")
    parser.add_argument("--no-language-detect", action="store_true",
                        help="Disable language detection (use original lang field from JSON)")
    args = parser.parse_args()

    input_dir = args.input_dir
    output_dir = args.output_dir
    num_workers = args.num_workers

    os.makedirs(output_dir, exist_ok=True)
    pattern = os.path.join(input_dir, "*.json.gz")
    files = glob.glob(pattern)

    if not files:
        print(f"No .json.gz files found in {input_dir}")
        sys.exit(1)

    # Prepare list of (input, output) pairs to process
    file_pairs = []
    for gz_file in files:
        base_name = os.path.basename(gz_file)
        csv_file = os.path.join(output_dir, base_name.replace('.json.gz', '.csv'))
        if os.path.exists(csv_file):
            print(f"Skipping {gz_file} - output file {csv_file} already exists")
            continue
        file_pairs.append((gz_file, csv_file))

    total_tweets = 0
    total_skipped = 0

    if not file_pairs:
        print("No files to process.")
        return

    print(f"Processing {len(file_pairs)} files with {num_workers} workers...")
    with multiprocessing.Pool(processes=num_workers) as pool:
        for result in pool.imap_unordered(process_json_file_worker, file_pairs):
            gz_file, tweets, skipped, error = result
            if error:
                print(f"Failed to process {gz_file}: {error}")
            else:
                total_tweets += tweets
                total_skipped += skipped

    print(f"Total: {total_tweets} tweets, {total_skipped} skipped")

if __name__ == "__main__":
    main() 