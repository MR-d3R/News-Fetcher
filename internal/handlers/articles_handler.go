package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"taskrunner/internal/models"
	"taskrunner/internal/repository"
	"taskrunner/logger"
	"taskrunner/pkg/rabbitmq"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// TaskHandler handles task-related HTTP requests
type ArticleHandler struct {
	RabbitChannel *amqp.Channel
	Repo          *repository.ArticleRepository
	logger        *logger.ColorfulLogger
	Ctx           context.Context
}

// NewTaskHandler creates a new instance of TaskHandler
func NewArticleHandler(ctx context.Context, rabbitChannel *amqp.Channel, repo *repository.ArticleRepository, logger *logger.ColorfulLogger) *ArticleHandler {
	return &ArticleHandler{
		RabbitChannel: rabbitChannel,
		Repo:          repo,
		Ctx:           ctx,
		logger:        logger,
	}
}

// FetchNewArticles creates task to fetch articles from provided source url
func (ah *ArticleHandler) FetchNewArticles(c *gin.Context) {
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
		ah.logger.Error("Failed to marshal task: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize task"})
		return
	}

	// Publish task to RabbitMQ
	err = rabbitmq.PublishMessage(ah.RabbitChannel, "content_fetch", taskJSON)
	if err != nil {
		ah.logger.Error("Failed to publish task: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue task"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"message": "Fetching new articles now",
	})
}

// GetArticleByID gets article info from DB
func (ah *ArticleHandler) GetArticleByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID format"})
		return
	}

	article, err := ah.Repo.GetArticleByID(c, id)
	if err != nil {
		ah.logger.Error("Failed to get article %d: %v", id, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to find article in database"})
	}

	c.JSON(http.StatusOK, article)
}

// GetArticleByID gets article info from DB
func (ah *ArticleHandler) GetRecentArticles(c *gin.Context) {
	articles, err := ah.Repo.GetRecentArticles(c, 50)
	if err != nil {
		ah.logger.Error("Failed to get recent articles: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to get recent articles"})
	}

	c.JSON(http.StatusOK, articles)
}

// RegisterRoutes registers all task routes
func (ah *ArticleHandler) RegisterArticleRoutes(router *gin.Engine) {
	router.POST("/article", ah.FetchNewArticles)
	router.GET("/article/:id", ah.GetArticleByID)
	router.GET("/article/recent", ah.GetRecentArticles)
}
