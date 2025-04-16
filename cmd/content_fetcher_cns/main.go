package main

import (
	"encoding/json"
	"fmt"
	"taskrunner/internal/config"
	"taskrunner/internal/models"
	"taskrunner/internal/utils"
	"taskrunner/logger"
	"taskrunner/pkg/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

func fetchContent(ch *amqp.Channel, logger *logger.ColorfulLogger, task models.FetchTask, newsApiKey string) error {
	body, err := utils.CreateRequest(task.SourceURL, "GET", "", newsApiKey)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	// После получения создаем задачу на обработку
	processTask := map[string]interface{}{
		"id":          task.ID,
		"content_raw": body,
		"source_url":  task.SourceURL,
	}

	taskJSON, _ := json.Marshal(processTask)

	// Отправляем задачу в очередь обработки
	return rabbitmq.PublishMessage(ch, "content_process", taskJSON)
}

func main() {
	cfg, err := config.InitConfig("FETCH CONTENT CONSUMER")
	if err != nil {
		panic(err)
	}
	logger := config.GetLogger()

	// Подключение к RabbitMQ
	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		logger.Panic("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		logger.Panic("Failed to open a channel: %v", err)
	}
	defer ch.Close()

	// Проверяем, существует ли очередь
	queue, err := ch.QueueDeclare(
		"content_fetch", // name
		true,            // wait, newse
		false,           // delete when unused
		false,           // exclusive
		false,           // no-wait
		nil,             // arguments
	)
	if err != nil {
		logger.Panic("Failed to declare a queue: %v", err)
	}

	logger.Info("Queue '%s' declared: %+v", "content_fetch", queue)

	// Счетчик количества сообщений в очереди
	queueInfo, err := ch.QueueInspect("content_fetch")
	if err != nil {
		logger.Panic("Failed to inspect queue: %v", err)
	}
	logger.Info("Queue '%s' has %d messages waiting", "content_fetch", queueInfo.Messages)

	// Настройка QoS (Quality of Service)
	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		logger.Panic("Failed to set QoS: %v", err)
	}

	msgs, err := ch.Consume(
		"content_fetch",
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		logger.Panic("Failed to register a consumer: %v", err)
	}

	logger.Info("Successfully registered consumer for queue '%s'", "content_fetch")

	newsAPIKey := cfg.NewsAPIKey
	// Обработка сообщений
	forever := make(chan bool)
	go func() {
		logger.Info("Starting message processing goroutine...")

		for d := range msgs {
			logger.Debug("Received a message: %s", d.Body)

			var task models.FetchTask
			if err := json.Unmarshal(d.Body, &task); err != nil {
				logger.Error("Error decoding task: %v", err)
				d.Nack(false, true) // Подтверждаем получение, даже если не смогли обработать
				continue
			}

			logger.Info("Processing task URL: %s", task.SourceURL)

			// status := checkURLStatus(task.URL)
			err := fetchContent(ch, logger, task, newsAPIKey)
			if err != nil {
				logger.Error("error processing task: %v", err)
				d.Nack(false, true) // отправляем обратно в очередь
			} else {
				d.Ack(false)
				logger.Info("Task %s completed and acknowledged", task.ID)
			}

		}

		logger.Info("Message channel closed, exiting goroutine")
	}()

	logger.Info("Consumer started. Waiting for messages... Press Ctrl+C to exit.")
	<-forever
}
