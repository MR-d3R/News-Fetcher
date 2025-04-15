package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"taskrunner/internal/models"
	"taskrunner/logger"
	"taskrunner/pkg/rabbitmq"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

// TaskHandler handles task-related HTTP requests
type TaskHandler struct {
	RabbitChannel *amqp.Channel
	RedisClient   *redis.Client
	logger        *logger.ColorfulLogger
	Ctx           context.Context
}

// NewTaskHandler creates a new instance of TaskHandler
func NewTaskHandler(ctx context.Context, rabbitChannel *amqp.Channel, redisClient *redis.Client, logger *logger.ColorfulLogger) *TaskHandler {
	return &TaskHandler{
		RabbitChannel: rabbitChannel,
		RedisClient:   redisClient,
		Ctx:           ctx,
		logger:        logger,
	}
}

// CreateTask handles creation of a new task
func (h *TaskHandler) CreateTask(c *gin.Context) {
	var input struct {
		URL string `json:"url" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input", "details": err.Error()})
		return
	}

	if input.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL cannot be empty"})
		return
	}

	// URL format validation
	if !strings.HasPrefix(input.URL, "http://") && !strings.HasPrefix(input.URL, "https://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL must start with http:// or https://"})
		return
	}

	// Create task with unique ID
	taskID := uuid.New().String()
	now := time.Now()

	task := models.FetchTask{
		ID:        taskID,
		Status:    "queued",
		SourceURL: input.URL,
		CreatedAt: now,
	}

	// Serialize task to JSON
	taskJSON, err := json.Marshal(task)
	if err != nil {
		h.logger.Error("Failed to marshal task: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize task"})
		return
	}

	// Publish task to RabbitMQ
	err = rabbitmq.PublishMessage(h.RabbitChannel, "content_fetch", taskJSON)
	if err != nil {
		h.logger.Error("Failed to publish task: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue task"})
		return
	}

	// Store task info in Redis
	taskData := map[string]interface{}{
		"source_url": input.URL,
		"status":     "processing",
		"created_at": now.Format(time.RFC3339),
	}

	err = h.RedisClient.HSet(h.Ctx, "task:"+taskID, taskData).Err()
	if err != nil {
		h.logger.Error("Failed to set Redis status: %v", err)
		// Continue anyway since the task is already in the queue
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"status":  "processing",
		"message": "Task submitted successfully",
	})
}

// GetTaskStatus retrieves status of a task by ID
func (h *TaskHandler) GetTaskStatus(c *gin.Context) {
	id := c.Param("id")

	// Validate ID format
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID format"})
		return
	}

	// Get task info from Redis
	result, err := h.RedisClient.HGetAll(h.Ctx, "task:"+id).Result()
	if err != nil || len(result) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAllTasks retrieves list of all tasks
func (h *TaskHandler) GetAllTasks(c *gin.Context) {
	// Use pattern to get all task keys
	keys, err := h.RedisClient.Keys(h.Ctx, "task:*").Result()
	if err != nil {
		h.logger.Error("Failed to fetch tasks: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
		return
	}

	if len(keys) == 0 {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	tasks := make(map[string]map[string]string)

	for _, key := range keys {
		taskID := key[5:] // Remove "task:" prefix
		result, err := h.RedisClient.HGetAll(h.Ctx, key).Result()
		if err == nil && len(result) > 0 {
			tasks[taskID] = result
		}
	}

	c.JSON(http.StatusOK, tasks)
}

// CancelTask cancels a task if it hasn't been completed
func (h *TaskHandler) CancelTask(c *gin.Context) {
	id := c.Param("id")

	// Validate ID format
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID format"})
		return
	}

	// Check task status
	status, err := h.RedisClient.HGet(h.Ctx, "task:"+id, "status").Result()
	if err != nil || status == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// Cannot cancel completed tasks
	if status == "done" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task already completed"})
		return
	}

	// Update status to cancelled
	err = h.RedisClient.HSet(h.Ctx, "task:"+id, "status", "cancelled").Err()
	if err != nil {
		h.logger.Error("Failed to cancel task: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel task"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "cancelled", "message": "Task cancelled successfully"})
}

// RegisterRoutes registers all task routes
func (h *TaskHandler) RegisterRoutes(router *gin.Engine) {
	router.POST("/task", h.CreateTask)
	router.GET("/task/:id", h.GetTaskStatus)
	router.GET("/tasks", h.GetAllTasks)
	router.POST("/task/:id/cancel", h.CancelTask)
}
