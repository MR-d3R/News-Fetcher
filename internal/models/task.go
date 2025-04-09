package models

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

type AppHandlers struct {
	rabbitChannel *amqp.Channel
	redisClient   *redis.Client
}

type Task struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Status string `json:"status"`
}
