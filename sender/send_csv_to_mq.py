#!/usr/bin/env python3
"""
Send CSV rows as messages to RabbitMQ.

Reads all CSV files in a specified directory (in sorted order) and sends each row as a message to the 'tweet_in' queue on RabbitMQ (localhost:5672).
"""

import os
import sys
import argparse
import pika
import glob


def send_csv_rows_to_mq(directory: str, queue_name: str = 'tweet_in'):
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
        print(f"Processing file: {csv_file}")
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
                    print(f"Sent {total_sent} messages...")
    print(f"Done. Sent {total_sent} messages in total.")
    connection.close()


def main():
    parser = argparse.ArgumentParser(description="Send CSV rows as messages to RabbitMQ queue 'tweet_in'.")
    parser.add_argument('directory', help='Directory containing CSV files to send')
    args = parser.parse_args()
    send_csv_rows_to_mq(args.directory)


if __name__ == '__main__':
    main()
