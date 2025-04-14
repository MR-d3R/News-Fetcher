package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"taskrunner/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

type Task struct {
	ID        string `json:"id"`
	SourceURL string `json:"source_url"`
}

// TaskHandler содержит обработчики для задач
type TaskHandler struct {
	RabbitChannel *amqp.Channel
	RedisClient   *redis.Client
	logger        *logger.ColorfulLogger
	Ctx           context.Context
}

// NewTaskHandler создает новый экземпляр обработчика задач
func NewTaskHandler(ctx context.Context, rabbitChannel *amqp.Channel, redisClient *redis.Client, logger *logger.ColorfulLogger) *TaskHandler {
	return &TaskHandler{
		RabbitChannel: rabbitChannel,
		RedisClient:   redisClient,
		Ctx:           ctx,
		logger:        logger,
	}
}

// CreateTask обрабатывает создание новой задачи
func (h *TaskHandler) CreateTask(c *gin.Context) {
	var input struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	if input.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL cannot be empty"})
		return
	}

	// Проверка, что URL имеет правильный формат
	if !strings.HasPrefix(input.URL, "http://") && !strings.HasPrefix(input.URL, "https://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL must start with http:// or https://"})
		return
	}

	taskID := uuid.New().String()
	task := Task{ID: taskID, SourceURL: input.URL}
	body, err := json.Marshal(task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize task"})
		return
	}

	err = h.RabbitChannel.Publish(
		"",              // exchange
		"content_fetch", // routing key
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish task"})
		return
	}

	// Установим статус в Redis
	err = h.RedisClient.HSet(h.Ctx, "task:"+taskID, "status", "processing").Err()
	if err != nil {
		h.logger.Error("Failed to set Redis status: %v", err)
		// В этом случае не возвращаем ошибку клиенту, так как задача уже поставлена в очередь
	}

	c.JSON(http.StatusOK, gin.H{"task_id": taskID})
}

// GetTaskStatus получает статус задачи по ID
func (h *TaskHandler) GetTaskStatus(c *gin.Context) {
	id := c.Param("id")
	result, err := h.RedisClient.HGetAll(h.Ctx, "task:"+id).Result()
	if err != nil || len(result) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAllTasks получает список всех задач
func (h *TaskHandler) GetAllTasks(c *gin.Context) {
	// Используем паттерн для получения всех ключей tasks:*
	keys, err := h.RedisClient.Keys(h.Ctx, "task:*").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tasks"})
		return
	}

	if len(keys) == 0 {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	tasks := make(map[string]map[string]string)

	for _, key := range keys {
		taskID := key[5:] // Удаляем префикс "task:"
		result, err := h.RedisClient.HGetAll(h.Ctx, key).Result()
		if err == nil && len(result) > 0 {
			tasks[taskID] = result
		}
	}

	c.JSON(http.StatusOK, tasks)
}

// CancelTask отменяет задачу, если она еще не выполнена
func (h *TaskHandler) CancelTask(c *gin.Context) {
	id := c.Param("id")

	// Проверяем статус задачи
	status, err := h.RedisClient.HGet(h.Ctx, "task:"+id, "status").Result()
	if err != nil || status == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	// Если задача уже выполнена, отмена невозможна
	if status == "done" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task already completed"})
		return
	}

	// Обновляем статус
	err = h.RedisClient.HSet(h.Ctx, "task:"+id, "status", "cancelled").Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel task"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// RegisterRoutes регистрирует все маршруты для задач
func (h *TaskHandler) RegisterRoutes(router *gin.Engine) {
	router.POST("/task", h.CreateTask)
	router.GET("/task/:id", h.GetTaskStatus)
	router.GET("/tasks", h.GetAllTasks)
	router.POST("/task/:id/cancel", h.CancelTask)
}
