#!/usr/bin/env python3

import pika

def test_queue_depth():
    try:
        # Connect to RabbitMQ
        connection = pika.BlockingConnection(pika.ConnectionParameters('localhost'))
        channel = connection.channel()
        
        # First, declare the queue normally
        channel.queue_declare(queue='tweet_in', durable=True)
        
        # Send a test message
        channel.basic_publish(
            exchange='',
            routing_key='tweet_in',
            body='test message',
            properties=pika.BasicProperties(delivery_mode=2)
        )
        print("Sent test message")
        
        # Now check the queue depth
        method = channel.queue_declare(queue='tweet_in', durable=True, passive=True)
        count = method.method.message_count
        print("Queue depth: {}".format(count))
        
        # Get queue info
        print("Queue method: {}".format(method))
        print("Queue method.method: {}".format(method.method))
        print("Queue method.method.message_count: {}".format(method.method.message_count))
        
        connection.close()
        
    except Exception as e:
        print("Error: {}".format(e))

if __name__ == '__main__':
    test_queue_depth() 