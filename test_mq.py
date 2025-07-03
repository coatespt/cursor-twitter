import pika

connection = pika.BlockingConnection(pika.ConnectionParameters('localhost'))
channel = connection.channel()
channel.queue_declare(queue='tweet_in', durable=True)

def callback(ch, method, properties, body):
    print("Received:", body.decode())

channel.basic_consume(queue='tweet_in', on_message_callback=callback, auto_ack=True)
print('Waiting for messages. To exit press CTRL+C')
channel.start_consuming()
