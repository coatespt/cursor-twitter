package main

import (
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Testing RabbitMQ connection...")

	// Connect to RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()
	fmt.Println("✓ Connected to RabbitMQ")

	// Create channel
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()
	fmt.Println("✓ Channel created")

	// Declare queue with same arguments as main application
	args := amqp.Table{
		"x-max-length":       int32(100000), // Maximum number of messages in queue
		"x-overflow":         "drop-head",   // Drop oldest messages when limit reached
		"x-max-length-bytes": int32(0),      // No byte limit (only message count)
	}
	q, err := ch.QueueDeclare(
		"tweet_in", // name
		true,       // durable
		false,      // delete when unused
		false,      // exclusive
		false,      // no-wait
		args,       // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}
	fmt.Printf("✓ Queue declared: %s\n", q.Name)

	// Set up consumer
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		log.Fatalf("Failed to register consumer: %v", err)
	}
	fmt.Println("✓ Consumer registered")

	// Listen for messages
	fmt.Println("Waiting for messages... (press Ctrl+C to stop)")
	
	messageCount := 0
	for msg := range msgs {
		messageCount++
		fmt.Printf("Received message %d: %s\n", messageCount, string(msg.Body))
		msg.Ack(false)
		
		if messageCount >= 5 {
			fmt.Println("Received 5 messages, stopping...")
			break
		}
	}

	fmt.Println("Test completed successfully!")
} 