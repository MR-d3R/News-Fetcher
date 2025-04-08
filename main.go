package main

import (
	"context"
	"fmt"
	"taskrunner/config"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.InitConfig("TaskRunner")
	if err != nil {
		panic(err)
	}
	logger := config.GetLogger()
	logger.Info("Запуск приложения TaskRunner")

	// Запись данных
	// db.Set(контекст, ключ, значение, время жизни в базе данных)
	if err := cfg.DB.Set(context.Background(), "key", "test value", 0).Err(); err != nil {
		logger.Error("failed to set data, error: %s", err.Error())
	}

	if err := cfg.DB.Set(context.Background(), "key2", 333, 30*time.Second).Err(); err != nil {
		logger.Error("failed to set data, error: %s", err.Error())
	}

	// Получение данных

	val, err := cfg.DB.Get(context.Background(), "key").Result()
	if err == redis.Nil {
		logger.Error("value not found")
	} else if err != nil {
		logger.Error("failed to get value, error: %v\n", err)
	}

	val2, err := cfg.DB.Get(context.Background(), "key2").Result()
	if err == redis.Nil {
		logger.Error("value not found")
	} else if err != nil {
		logger.Error("failed to get value, error: %v\n", err)
	}

	fmt.Printf("value: %v\n", val)
	fmt.Printf("value: %v\n", val2)
}
