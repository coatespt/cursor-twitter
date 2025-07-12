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
from atomic_file import update_sender_status, get_sender_status


def get_queue_depth(channel, queue_name):
    """Get the current number of messages in the queue."""
    try:
        method = channel.queue_declare(queue=queue_name, durable=True, passive=True)
        return method.method.message_count
    except Exception as e:
        print(f"Warning: Could not get queue depth: {e}")
        return 0


def send_csv_rows_to_mq(directory, status_file=None, queue_name='tweet_in', max_queue_depth=10000, pause_duration=1.0):
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

    print(f"Flow control enabled: max queue depth = {max_queue_depth}, pause duration = {pause_duration}s")
    
    total_sent = 0
    flow_control_pauses = 0
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
                
                # Check queue depth before sending
                queue_depth = get_queue_depth(channel, queue_name)
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
                    queue_depth = get_queue_depth(channel, queue_name)
                    print(f"Sent {total_sent} messages... (queue depth: {queue_depth})", end='\r', flush=True)
    
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
