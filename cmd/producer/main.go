package main

import (
	"context"

	"taskrunner/internal/config"
	"taskrunner/internal/handlers"
	"taskrunner/pkg/rabbitmq"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	cfg, err := config.InitConfig("PRODUCER")
	if err != nil {
		panic(err)
	}
	logger := config.GetLogger()

	// Подключение к RabbitMQ
	conn, ch, err := rabbitmq.SetupRabbitMQ(cfg.RabbitMQURL, cfg.QueueName)
	if err != nil {
		logger.Error("Failed to setup RabbitMQ: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	// Подключение к Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	defer rdb.Close()

	// Проверка соединения с Redis
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		logger.Error("Failed to connect to Redis: %v", err)
	}

	// Создание обработчиков
	taskHandler := handlers.NewTaskHandler(ctx, ch, rdb, logger)

	// Создание Gin роутера
	r := gin.Default()

	// Регистрация маршрутов
	taskHandler.RegisterRoutes(r)

	// Запуск HTTP-сервера
	logger.Info("Starting producer server on http://localhost%s", cfg.ServerPort)
	if err := r.Run(cfg.ServerPort); err != nil {
		logger.Error("Failed to start server: %v", err)
	}
}
