#!/usr/bin/env python3
"""
Send a small number of CSV rows as messages to RabbitMQ for testing.
"""

import os
import sys
import pika
import glob

def send_test_tweets(directory: str, max_tweets: int = 500, queue_name: str = 'tweet_in'):
    # Connect to RabbitMQ
    connection = pika.BlockingConnection(pika.ConnectionParameters('localhost'))
    channel = connection.channel()
    channel.queue_declare(queue=queue_name, durable=True)

    # Find all CSV files in the directory, sorted
    csv_files = sorted(glob.glob(os.path.join(directory, '*.csv')))
    if not csv_files:
        print(f"No CSV files found in {directory}")
        return

    total_sent = 0
    for csv_file in csv_files:
        if total_sent >= max_tweets:
            break
            
        print(f"Processing file: {csv_file}")
        with open(csv_file, 'r', encoding='utf-8') as f:
            for line_num, line in enumerate(f, 1):
                if total_sent >= max_tweets:
                    break
                    
                line = line.rstrip('\n')
                if not line:
                    continue
                    
                # Skip header rows
                if line.startswith('id_str,created_at'):
                    continue
                    
                channel.basic_publish(
                    exchange='',
                    routing_key=queue_name,
                    body=line.encode('utf-8'),
                    properties=pika.BasicProperties(delivery_mode=2)  # make message persistent
                )
                total_sent += 1
                if total_sent % 100 == 0:
                    print(f"Sent {total_sent} messages...")
                    
    print(f"Done. Sent {total_sent} messages in total.")
    connection.close()

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 send_test_tweets.py <directory> [max_tweets]")
        sys.exit(1)
        
    directory = sys.argv[1]
    max_tweets = 500  # Default
    if len(sys.argv) > 2:
        max_tweets = int(sys.argv[2])
        
    send_test_tweets(directory, max_tweets)

if __name__ == '__main__':
    main() 