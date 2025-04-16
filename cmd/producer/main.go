package main

import (
	"context"
	"log"

	"taskrunner/internal/config"
	"taskrunner/internal/handlers"
	"taskrunner/internal/repository"
	"taskrunner/pkg/rabbitmq"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
)

func main() {
	ctx := context.Background()
	cfg, err := config.InitConfig("NEWS API PRODUCER")
	if err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}
	logger := config.GetLogger()
	logger.Info("Starting News API Producer service")

	// Set up RabbitMQ connection and channel
	queueNames := []string{"tasks", "content_fetch", "content_process"}
	conn, ch, err := rabbitmq.SetupRabbitMQ(cfg.RabbitMQURL, queueNames)
	if err != nil {
		logger.Panic("Failed to setup RabbitMQ: %v", err)
	}
	defer conn.Close()
	defer ch.Close()
	logger.Info("All th e queues declared successfully")

	// Test Redis connection
	pong, err := cfg.DB.Ping(ctx).Result()
	if err != nil {
		logger.Panic("Failed to connect to Redis: %v", err)
	}
	logger.Info("Redis connection successful: %s", pong)
	defer cfg.DB.Close()

	// Article handler initializtion
	pool, err := pgxpool.Connect(ctx, cfg.PostgresAddr)
	if err != nil {
		logger.Panic("Failed to connect to Postgres")
	}
	repo := repository.NewArticleRepository(pool, cfg.DB)
	_, err = pool.Exec(ctx, config.Schema)
	if err != nil {
		logger.Panic("Failed to initialize tables: %v", err)
	} else {
		logger.Info("Tables initialized successfully")
	}
	articleHandler := handlers.NewArticleHandler(ctx, ch, repo, logger)

	router := gin.Default()

	// Add CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "news-api-producer",
		})
	})

	// Register task routes
	articleHandler.RegisterArticleRoutes(router)

	// Start HTTP server
	logger.Info("Starting producer server on http://localhost%s", cfg.ServerPort)
	if err := router.Run(cfg.ServerPort); err != nil {
		logger.Panic("Failed to start server: %v", err)
	}
}
