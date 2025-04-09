package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

// SetupRabbitMQ устанавливает соединение с RabbitMQ и создает канал
func SetupRabbitMQ(url string, queueName string) (*amqp.Connection, *amqp.Channel, error) {
	// Подключение к RabbitMQ
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, err
	}

	// Объявление очереди
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
		return nil, nil, err
	}

	return conn, ch, nil
}
