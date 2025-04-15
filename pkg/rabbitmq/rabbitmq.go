package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// SetupRabbitMQ establishes connection to RabbitMQ and creates a channel
func SetupRabbitMQ(rabbitMQURL, queueName string) (*amqp.Connection, *amqp.Channel, error) {
	// Connect to RabbitMQ
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to RabbitMQ: %v", err)
	}

	// Create channel
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to open a channel: %v", err)
	}

	// Ensure main queue exists
	if queueName != "" {
		_, err = ch.QueueDeclare(
			queueName, // name
			true,      // durable
			false,     // delete when unused
			false,     // exclusive
			false,     // no-wait
			nil,       // arguments
		)
		if err != nil {
			ch.Close()
			conn.Close()
			return nil, nil, fmt.Errorf("failed to declare queue %s: %v", queueName, err)
		}
	}

	return conn, ch, nil
}

// PublishMessage sends a message to the specified queue
func PublishMessage(ch *amqp.Channel, queueName string, body []byte) error {
	return ch.Publish(
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // Make message persistent
			Body:         body,
		})
}
