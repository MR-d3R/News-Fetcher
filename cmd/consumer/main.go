package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"taskrunner/internal/config"
	"taskrunner/internal/models"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

func checkURLStatus(url string) string {
	// Проверка на пустой URL
	if url == "" {
		return "error: empty URL"
	}

	// Проверка схемы URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "error: URL must start with http:// or https://"
	}

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return fmt.Sprintf("status: %d", resp.StatusCode)
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
		logger.Error("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		logger.Error("Failed to open a channel: %v", err)
	}
	defer ch.Close()

	// Проверяем, существует ли очередь
	queue, err := ch.QueueDeclare(
		cfg.QueueName, // name
		true,          // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	if err != nil {
		logger.Error("Failed to declare a queue: %v", err)
	}

	logger.Info("Queue '%s' declared: %+v", cfg.QueueName, queue)

	// Счетчик количества сообщений в очереди
	queueInfo, err := ch.QueueInspect(cfg.QueueName)
	if err != nil {
		logger.Error("Failed to inspect queue: %v", err)
	}
	logger.Info("Queue '%s' has %d messages waiting", cfg.QueueName, queueInfo.Messages)

	// Настройка QoS (Quality of Service)
	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		logger.Error("Failed to set QoS: %v", err)
	}

	// Подписка на очередь
	msgs, err := ch.Consume(
		cfg.QueueName, // queue
		"",            // consumer (пустая строка = автогенерация имени)
		false,         // auto-ack (ИЗМЕНЕНО на false - ручное подтверждение)
		false,         // exclusive
		false,         // no-local
		false,         // no-wait
		nil,           // args
	)
	if err != nil {
		logger.Error("Failed to register a consumer: %v", err)
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
		logger.Error("Failed to connect to Redis: %v", err)
	}
	logger.Info("Redis connection successful: %s", pong)

	// Обработка сообщений
	forever := make(chan bool)
	go func() {
		logger.Info("Starting message processing goroutine...")

		for d := range msgs {
			logger.Debug("Received a message: %s", d.Body)

			var task models.Task
			if err := json.Unmarshal(d.Body, &task); err != nil {
				logger.Error("Error decoding task: %v", err)
				d.Ack(false) // Подтверждаем получение, даже если не смогли обработать
				continue
			}

			logger.Info("Processing task %s: %s", task.ID, task.URL)

			if task.URL == "" {
				// Сохраняем ошибку в Redis
				err := rdb.HSet(ctx, "task:"+task.ID,
					"status", "error",
					"result", "empty URL provided",
				).Err()
				if err != nil {
					logger.Error("Error saving error result to Redis: %v", err)
				}

				// Устанавливаем TTL
				rdb.Expire(ctx, "task:"+task.ID, time.Hour)

				// Подтверждаем обработку сообщения
				d.Ack(false)
				logger.Info("Task %s failed: empty URL", task.ID)
				continue
			}

			status := checkURLStatus(task.URL)
			logger.Debug("URL check result: %s", status)

			// Сохраняем результат
			err := rdb.HSet(ctx, "task:"+task.ID,
				"status", "done",
				"result", status,
				"completed_at", time.Now().Format(time.RFC3339),
			).Err()
			if err != nil {
				logger.Error("Error saving result to Redis: %v", err)
			} else {
				logger.Info("Result saved to Redis for task %s", task.ID)
			}

			// Устанавливаем TTL (например, 1 час)
			err = rdb.Expire(ctx, "task:"+task.ID, time.Hour).Err()
			if err != nil {
				logger.Error("Error setting TTL: %v", err)
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
