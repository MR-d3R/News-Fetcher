package main

import (
	"context"
	"log"

	"taskrunner/internal/config"
	"taskrunner/internal/handlers"
	"taskrunner/pkg/rabbitmq"

	"github.com/gin-gonic/gin"
)

func main() {
	// Create context
	ctx := context.Background()

	// Initialize configuration
	cfg, err := config.InitConfig("NEWS API PRODUCER")
	if err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}
	logger := config.GetLogger()
	logger.Info("Starting News API Producer service")

	// Set up RabbitMQ connection and channel
	conn, ch, err := rabbitmq.SetupRabbitMQ(cfg.RabbitMQURL, "tasks")
	if err != nil {
		logger.Panic("Failed to setup RabbitMQ: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	// Declare necessary queues
	queueNames := []string{"content_fetch", "content_process"}
	for _, qName := range queueNames {
		_, err := ch.QueueDeclare(
			qName, // name
			true,  // durable
			false, // delete when unused
			false, // exclusive
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			logger.Panic("Failed to declare queue %s: %v", qName, err)
		}
		logger.Info("Queue '%s' declared successfully", qName)
	}

	// Test Redis connection
	pong, err := cfg.DB.Ping(ctx).Result()
	if err != nil {
		logger.Panic("Failed to connect to Redis: %v", err)
	}
	logger.Info("Redis connection successful: %s", pong)
	defer cfg.DB.Close()

	// Create handler
	taskHandler := handlers.NewTaskHandler(ctx, ch, cfg.DB, logger)

	// Configure Gin router
	router := gin.Default()

	// Add CORS middleware if needed
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

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "news-api-producer",
		})
	})

	// Register task routes
	taskHandler.RegisterRoutes(router)

	// Start HTTP server
	logger.Info("Starting producer server on http://localhost%s", cfg.ServerPort)
	if err := router.Run(cfg.ServerPort); err != nil {
		logger.Panic("Failed to start server: %v", err)
	}
}
