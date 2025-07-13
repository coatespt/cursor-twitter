#!/usr/bin/env python3
"""
Send CSV rows as messages to RabbitMQ.

Reads all CSV files in a specified directory (in sorted order) and sends each row as a message to the 'tweet_in' queue on RabbitMQ (localhost:5672).
Tracks the last processed file in a status file to enable resuming from where it left off.
Includes flow control to pause sending when the queue gets too large.
"""

import os
import sys
import argparse
import pika
import glob
import yaml
import time
import csv
import io
from atomic_file import update_sender_status, get_sender_status


def get_queue_depth(queue_name):
    """Get the current number of messages in the queue using RabbitMQ management API."""
    try:
        import requests
        from requests.auth import HTTPBasicAuth
        
        url = f'http://localhost:15672/api/queues/%2f/{queue_name}'
        response = requests.get(url, auth=HTTPBasicAuth('guest', 'guest'), timeout=2)
        if response.status_code == 200:
            data = response.json()
            count = data.get('messages', 0)
            return count
        else:
            print(f"[DEBUG] Management API returned status {response.status_code}: {response.text}")
            return 0
            
    except ImportError:
        print("[DEBUG] requests library not available, falling back to pika")
        # Fallback to pika method
        try:
            connection_params = pika.ConnectionParameters(
                host='localhost',
                connection_attempts=2,
                retry_delay=1,
                socket_timeout=2.0
            )
            connection = pika.BlockingConnection(connection_params)
            channel = connection.channel()
            
            method = channel.queue_declare(queue=queue_name, durable=True, passive=True)
            count = method.method.message_count
            
            connection.close()
            return count
            
        except Exception as e:
            print(f"Warning: Pika fallback also failed: {e}")
            return 0
            
    except Exception as e:
        print(f"Warning: Could not get queue depth: {e}")
        return 0


def send_csv_rows_to_mq(directory, status_file=None, queue_name='tweet_in', max_queue_depth=10000, pause_duration=1.0):
    # Connect to RabbitMQ with timeout
    connection_params = pika.ConnectionParameters(
        host='localhost',
        connection_attempts=3,
        retry_delay=1,
        socket_timeout=5.0  # 5 second timeout
    )
    connection = pika.BlockingConnection(connection_params)
    channel = connection.channel()
    
    # Set queue size limits to enable flow control
    queue_args = {
        'x-max-length': 100000,        # Maximum number of messages in queue
        'x-overflow': 'drop-head',     # Drop oldest messages when limit reached
        'x-max-length-bytes': 0        # No byte limit (only message count)
    }
    channel.queue_declare(queue=queue_name, durable=True, arguments=queue_args)

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

    print(f"Flow control enabled: max queue depth = {max_queue_depth}, pause duration = {pause_duration}s")
    
    total_sent = 0
    flow_control_pauses = 0
    for i, csv_file in enumerate(csv_files[start_index:], start_index):
        print(f"Processing file {i+1}/{len(csv_files)}: {os.path.basename(csv_file)}")
        
        # Update status file with current file being processed
        if status_file:
            update_sender_status(status_file, csv_file)
        
        with open(csv_file, 'r', encoding='utf-8') as f:
            reader = csv.reader(f)
            # Skip header row if it exists
            try:
                first_row = next(reader)
                if first_row[0] == 'id_str':
                    print(f"Skipping header row in {os.path.basename(csv_file)}")
                else:
                    # Not a header, process this row
                    line = ','.join(first_row)
                    if line.strip():
                        # Check queue depth more frequently (every 1000 messages) to prevent overflow
                        if total_sent % 1000 == 0:
                            queue_depth = get_queue_depth(queue_name)
                            if queue_depth >= max_queue_depth:
                                if flow_control_pauses % 10 == 0:  # Log every 10th pause to avoid spam
                                    print(f"Queue depth {queue_depth} >= {max_queue_depth}, pausing for {pause_duration}s...")
                                time.sleep(pause_duration)
                                flow_control_pauses += 1
                                continue
                        
                        channel.basic_publish(
                            exchange='',
                            routing_key=queue_name,
                            body=line.encode('utf-8'),
                            properties=pika.BasicProperties(delivery_mode=2)  # make message persistent
                        )
                        total_sent += 1
                        if total_sent % 1000 == 0:
                            queue_depth = get_queue_depth(queue_name)
                            print(f"Sent {total_sent} messages... (queue depth: {queue_depth}) [DEBUG: raw_value={queue_depth}]", end='\r', flush=True)
            except StopIteration:
                # Empty file
                continue
            
            # Process remaining rows
            for line_num, row in enumerate(reader, 2):  # Start from 2 since we already processed the first row
                line = ','.join(row)
                if not line.strip():
                    continue
                
                # Check queue depth more frequently (every 1000 messages) to prevent overflow
                if total_sent % 1000 == 0:
                    queue_depth = get_queue_depth(queue_name)
                    if queue_depth >= max_queue_depth:
                        if flow_control_pauses % 10 == 0:  # Log every 10th pause to avoid spam
                            print(f"Queue depth {queue_depth} >= {max_queue_depth}, pausing for {pause_duration}s...")
                        time.sleep(pause_duration)
                        flow_control_pauses += 1
                        continue
                
                channel.basic_publish(
                    exchange='',
                    routing_key=queue_name,
                    body=line.encode('utf-8'),
                    properties=pika.BasicProperties(delivery_mode=2)  # make message persistent
                )
                total_sent += 1
                if total_sent % 1000 == 0:
                    queue_depth = get_queue_depth(queue_name)
                    print(f"Sent {total_sent} messages... (queue depth: {queue_depth}) [DEBUG: raw_value={queue_depth}]", end='\r', flush=True)
                

    
    print(f"\nDone. Sent {total_sent} messages in total.")
    if flow_control_pauses > 0:
        print(f"Flow control triggered {flow_control_pauses} times")
    connection.close()


def main():
    parser = argparse.ArgumentParser(description="Send CSV rows as messages to RabbitMQ queue 'tweet_in'.")
    parser.add_argument('directory', help='Directory containing CSV files to send')
    parser.add_argument('--config', default='../config/config.yaml', help='Path to config file')
    parser.add_argument('--max-queue-depth', type=int, 
                       help='Maximum queue depth before pausing (overrides config)')
    parser.add_argument('--pause-duration', type=float,
                       help='Duration to pause when queue is full, in seconds (overrides config)')
    args = parser.parse_args()
    
    # Load config file
    status_file = None
    max_queue_depth = 10000  # Default fallback
    pause_duration = 1.0     # Default fallback
    
    try:
        with open(args.config, 'r') as f:
            config = yaml.safe_load(f)
            
            # Get sender settings from config
            sender_config = config.get('sender', {})
            status_file = sender_config.get('status_file')
            max_queue_depth = sender_config.get('max_queue_depth', 10000)
            pause_duration = sender_config.get('pause_duration', 1.0)
            
            if status_file:
                print(f"Status tracking enabled: {status_file}")
            else:
                print("Status tracking disabled (no status_file in config)")
                
            print(f"Config loaded: max_queue_depth={max_queue_depth}, pause_duration={pause_duration}")
            
    except Exception as e:
        print(f"Warning: Could not load config file {args.config}: {e}")
        print("Continuing with default values")
    
    # Command line arguments override config values
    if args.max_queue_depth is not None:
        max_queue_depth = args.max_queue_depth
        print(f"Command line override: max_queue_depth={max_queue_depth}")
    
    if args.pause_duration is not None:
        pause_duration = args.pause_duration
        print(f"Command line override: pause_duration={pause_duration}")
    
    send_csv_rows_to_mq(args.directory, status_file, max_queue_depth=max_queue_depth, 
                       pause_duration=pause_duration)


if __name__ == '__main__':
    main()
