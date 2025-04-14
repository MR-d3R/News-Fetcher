package main

import (
	"context"
	"encoding/json"
	"fmt"
	"taskrunner/internal/config"
	"taskrunner/internal/utils"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

type FetchTask struct {
	ID         string `json:"id"`
	SourceURL  string `json:"source_url"`
	SourceType string `json:"source_type"` // rss, api, etc
}

func fetchContent(ctx context.Context, ch *amqp.Channel, task FetchTask, newsApiKey string) error {
	// url := "https://newsapi.org/v2/everything?q=tesla&from=2025-03-09&sortBy=publishedAt"
	body, err := utils.CreateRequest(task.SourceURL, "GET", "", newsApiKey)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	// После получения создаем задачу на обработку
	processTask := map[string]interface{}{
		"content_raw": body,
		"source_url":  task.SourceURL,
	}

	taskJSON, _ := json.Marshal(processTask)

	// Отправляем задачу в очередь обработки
	return ch.Publish(
		"",                // exchange
		"content_process", // routing key
		false,             // mandatory
		false,             // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        taskJSON,
		})
}

func main() {
	ctx := context.Background()
	cfg, err := config.InitConfig("CONSUMER")
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
		cfg.QueueName, // name
		true,          // wait, newse
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	if err != nil {
		logger.Panic("Failed to declare a queue: %v", err)
	}

	logger.Info("Queue '%s' declared: %+v", cfg.QueueName, queue)

	// Счетчик количества сообщений в очереди
	queueInfo, err := ch.QueueInspect(cfg.QueueName)
	if err != nil {
		logger.Panic("Failed to inspect queue: %v", err)
	}
	logger.Info("Queue '%s' has %d messages waiting", cfg.QueueName, queueInfo.Messages)

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

	logger.Info("Successfully registered consumer for queue '%s'", cfg.QueueName)

	// Подключение к Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	defer rdb.Close()

	// Проверка соединения с Redis
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		logger.Panic("Failed to connect to Redis: %v", err)
	}
	logger.Info("Redis connection successful: %s", pong)

	newsAPIKey := cfg.NewsAPIKey
	// Обработка сообщений
	forever := make(chan bool)
	go func() {
		logger.Info("Starting message processing goroutine...")

		for d := range msgs {
			logger.Debug("Received a message: %s", d.Body)

			var task FetchTask
			if err := json.Unmarshal(d.Body, &task); err != nil {
				logger.Error("Error decoding task: %v", err)
				d.Nack(false, false) // Подтверждаем получение, даже если не смогли обработать
				continue
			}

			logger.Info("Processing task URL: %s", task.SourceURL)

			// status := checkURLStatus(task.URL)
			err := fetchContent(context.Background(), ch, task, newsAPIKey)
			if err != nil {
				logger.Error("error processing task: %v", err)
				d.Nack(false, true) // отправляем обратно в очередь
			} else {
				d.Ack(false) // подтверждаем выполнение
				logger.Debug("info from URL %s successfully fetched", task.ID)
			}

			// Подтверждаем обработку сообщения
			d.Ack(false)
			logger.Info("Task %s completed and acknowledged", task.ID)
		}

		logger.Info("Message channel closed, exiting goroutine")
	}()

	logger.Info("Consumer started. Waiting for messages... Press Ctrl+C to exit.")
	<-forever
}
