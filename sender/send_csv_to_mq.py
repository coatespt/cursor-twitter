#!/usr/bin/env python3
"""
Send CSV rows as messages to RabbitMQ.

Reads all CSV files in a specified directory (in sorted order) and sends each row as a message to the 'tweet_in' queue on RabbitMQ (localhost:5672).
Tracks the last processed file in a status file to enable resuming from where it left off.
"""

import os
import sys
import argparse
import pika
import glob
import yaml
from atomic_file import update_sender_status, get_sender_status


def send_csv_rows_to_mq(directory, status_file=None, queue_name='tweet_in'):
    # Connect to RabbitMQ
    connection = pika.BlockingConnection(pika.ConnectionParameters('localhost'))
    channel = connection.channel()
    channel.queue_declare(queue=queue_name, durable=True)

    # Find all CSV files in the directory, sorted
    csv_files = sorted(glob.glob(os.path.join(directory, '*.csv')))
    if not csv_files:
        print(f"No CSV files found in {directory}")
        return

    # Check if we should resume from a previous run
    start_index = 0
    if status_file:
        last_processed = get_sender_status(status_file)
        if last_processed:
            try:
                start_index = csv_files.index(last_processed) + 1
                print(f"Resuming from file {start_index + 1} of {len(csv_files)} (last processed: {last_processed})")
            except ValueError:
                print(f"Last processed file '{last_processed}' not found in directory, starting from beginning")
        else:
            print("No previous status found, starting from beginning")

    total_sent = 0
    for i, csv_file in enumerate(csv_files[start_index:], start_index):
        print(f"Processing file {i+1}/{len(csv_files)}: {os.path.basename(csv_file)}")
        
        # Update status file with current file being processed
        if status_file:
            update_sender_status(status_file, csv_file)
        
        with open(csv_file, 'r', encoding='utf-8') as f:
            for line_num, line in enumerate(f, 1):
                line = line.rstrip('\n')
                if not line:
                    continue
                channel.basic_publish(
                    exchange='',
                    routing_key=queue_name,
                    body=line.encode('utf-8'),
                    properties=pika.BasicProperties(delivery_mode=2)  # make message persistent
                )
                total_sent += 1
                if total_sent % 1000 == 0:
                    print(f"Sent {total_sent} messages...", end='\r', flush=True)
    
    print(f"\nDone. Sent {total_sent} messages in total.")
    connection.close()


def main():
    parser = argparse.ArgumentParser(description="Send CSV rows as messages to RabbitMQ queue 'tweet_in'.")
    parser.add_argument('directory', help='Directory containing CSV files to send')
    parser.add_argument('--config', default='../config/config.yaml', help='Path to config file')
    args = parser.parse_args()
    
    # Load config file
    status_file = None
    try:
        with open(args.config, 'r') as f:
            config = yaml.safe_load(f)
            status_file = config.get('sender', {}).get('status_file')
            if status_file:
                print(f"Status tracking enabled: {status_file}")
            else:
                print("Status tracking disabled (no status_file in config)")
    except Exception as e:
        print(f"Warning: Could not load config file {args.config}: {e}")
        print("Continuing without status tracking")
    
    send_csv_rows_to_mq(args.directory, status_file)


if __name__ == '__main__':
    main()
