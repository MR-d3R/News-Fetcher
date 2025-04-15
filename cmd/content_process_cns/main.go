package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"taskrunner/internal/config"
	"taskrunner/internal/repository"

	"github.com/jackc/pgx/v4/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

type ProcessTask struct {
	ID         string `json:"id"`
	ContentRaw string `json:"content_raw"`
	SourceURL  string `json:"source_url"`
}

type NewsAPIResp struct {
	Status   string               `json:"status"`
	Articles []repository.Article `json:"articles"`
}

type Category struct {
	Programming    []string
	Politics       []string
	Cryptocurrency []string
}

func NewCategory() *Category {
	return &Category{
		Programming:    []string{"golang", "python", "javascript", "typescript", "html", "ai", "css", "js", "docker", "api"},
		Politics:       []string{"politics", "government", "putin", "tramp"},
		Cryptocurrency: []string{"bitcoin", "crypto", "btc", "ethereum", "eth"},
	}
}

func processNewsAPI(rawContent *string) ([]repository.Article, error) {
	// Декодирование из base64
	decodedData, err := base64.StdEncoding.DecodeString(*rawContent)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 content: %v", err)
	}

	var response NewsAPIResp
	err = json.Unmarshal(decodedData, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse json from news api: %v", err)
	}

	if response.Status == "ok" {
		return response.Articles, nil
	} else {
		return nil, fmt.Errorf("news api status not ok: %s", response.Status)
	}
}
func processContent(ctx context.Context, repo *repository.ArticleRepository, task ProcessTask) error {
	sourceType := determineSourceType(task.SourceURL)

	var articles []repository.Article
	categories := NewCategory()

	switch sourceType {
	case "api":
		var err error
		articles, err = processNewsAPI(&task.ContentRaw)
		if err != nil {
			return fmt.Errorf("failed to process news api: %v", err)
		}
	case "rss":
		var feed struct {
			Title   string `xml:"channel>item>title"`
			Content string `xml:"channel>item>description"`
		}
		if err := xml.Unmarshal([]byte(task.ContentRaw), &feed); err != nil {
			return err
		}

	}

	for _, article := range articles {
		determinedCategory := categorizeContent(article.Description, categories)
		article.Category = determinedCategory

		// Сохраняем в БД и кэш
		if err := repo.SaveArticle(ctx, &article); err != nil {
			return err
		}
	}

	return nil
}

// Определение типа источника по ссылке
func determineSourceType(url string) string {
	if strings.Contains(url, "rss") || strings.Contains(url, "feed") {
		return "rss"
	}
	if strings.Contains(url, "api") {
		return "api"
	}
	return "html"
}

// Определение категории по ключевым словам
func categorizeContent(content string, categories *Category) string {
	contentLower := strings.ToLower(content)

	if containsAny(contentLower, categories.Programming) {
		return "programming"
	}
	if containsAny(contentLower, categories.Politics) {
		return "politics"
	}
	if containsAny(contentLower, categories.Cryptocurrency) {
		return "cryptocurrency"
	}

	return "general"
}

func containsAny(s string, words []string) bool {
	for _, word := range words {
		// Создаем шаблон для сопоставления слова как целого слова
		pattern := `\b` + regexp.QuoteMeta(strings.ToLower(word)) + `\b`
		matched, _ := regexp.MatchString(pattern, s)
		if matched {
			return true
		}
	}
	return false
}

func main() {
	ctx := context.Background()
	cfg, err := config.InitConfig("PROCESS CONTENT CONSUMER")
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
		"content_process", // name
		true,              // wait, newse
		false,             // delete when unused
		false,             // exclusive
		false,             // no-wait
		nil,               // arguments
	)
	if err != nil {
		logger.Panic("Failed to declare a queue: %v", err)
	}

	logger.Info("Queue '%s' declared: %+v", "content_process", queue)

	// Счетчик количества сообщений в очереди
	queueInfo, err := ch.QueueInspect("content_process")
	if err != nil {
		logger.Panic("Failed to inspect queue: %v", err)
	}
	logger.Info("Queue '%s' has %d messages waiting", "content_process", queueInfo.Messages)

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
		"content_process",
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
	logger.Info("Successfully registered consumer for queue '%s'", "content_process")

	// Проверка соединения с Redis
	pong, err := cfg.DB.Ping(ctx).Result()
	if err != nil {
		logger.Panic("Failed to connect to Redis: %v", err)
	}
	logger.Info("Redis connection successful: %s", pong)

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

	// Обработка сообщений
	forever := make(chan bool)
	go func() {
		logger.Info("Starting message processing goroutine...")

		for d := range msgs {
			logger.Debug("Received a message: %s", d.Body)

			var task ProcessTask
			if err := json.Unmarshal(d.Body, &task); err != nil {
				logger.Error("Error decoding task: %v", err)
				d.Nack(false, false)
				continue
			}

			logger.Info("Processing task %s: %s", task.ID, task.SourceURL)

			err := processContent(context.Background(), repo, task)
			if err != nil {
				logger.Error("error processing task: %v", err)
				d.Nack(false, true) // отправляем обратно в очередь
			} else {
				d.Ack(false)
				logger.Debug("info from URL %s successfully fetched", task.ID)
			}

			d.Ack(false)
			logger.Info("Task %s completed and acknowledged", task.ID)
		}

		logger.Info("Message channel closed, exiting goroutine")
	}()

	logger.Info("Consumer started. Waiting for messages... Press Ctrl+C to exit.")
	<-forever
}
