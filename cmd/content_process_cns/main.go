package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"taskrunner/internal/config"
	"taskrunner/internal/repository"
	"taskrunner/logger"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/net/html"
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

type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Items       []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"dc:creator"`
	Category    string `xml:"category"`
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

func processNewsAPI(data []byte) ([]repository.Article, error) {
	if strings.HasPrefix(string(data), "<") || strings.HasPrefix(string(data), "<html") || strings.HasPrefix(string(data), "<!DOCTYPE") {
		return nil, fmt.Errorf("received HTML instead of JSON. API call likely failed with a 404 error. Check your API URL and key")
	}

	var response NewsAPIResp
	err := json.Unmarshal(data, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse json from news api: %v", err)
	}

	if response.Status == "ok" {
		return response.Articles, nil
	} else {
		return nil, fmt.Errorf("news api status not ok: %s", response.Status)
	}
}
func processContent(ctx context.Context, logger *logger.ColorfulLogger, repo *repository.ArticleRepository, task ProcessTask) error {
	sourceType := determineSourceType(task.SourceURL)
	decodedData, err := base64.StdEncoding.DecodeString(task.ContentRaw)
	if err != nil {
		return fmt.Errorf("failed to decode base64 content: %v", err)
	}

	var articles []repository.Article

	switch sourceType {
	case "api":
		var err error
		articles, err = processNewsAPI(decodedData)
		if err != nil {
			return fmt.Errorf("failed to process news api: %v", err)
		}
	case "rss":
		var rss RSS
		if err := xml.Unmarshal(decodedData, &rss); err != nil {
			return fmt.Errorf("failed to parse RSS feed: %v", err)
		}

		for _, item := range rss.Channel.Items {
			var publishedTime time.Time
			var err error
			if item.PubDate != "" {
				publishedTime, err = parseRSSDate(item.PubDate)
				if err != nil {
					fmt.Printf("Warning: Could not parse date '%s': %v\n", item.PubDate, err)
					publishedTime = time.Now()
				}
			} else {
				publishedTime = time.Now()
			}
			formattedDate := publishedTime.Format(time.RFC3339)

			imageURL := extractImageURLWithParser(item.Description)

			article := repository.Article{
				Author:      item.Author,
				Title:       item.Title,
				Description: item.Description,
				Content:     item.Description,
				URL:         item.Link,
				ImageURL:    imageURL,
				Category:    item.Category,
				PublishedAt: formattedDate,
			}
			articles = append(articles, article)
		}
	}

	categories := NewCategory()
	for _, article := range articles {
		if article.Category == "" {
			determinedCategory := categorizeContent(article.Description, categories)
			article.Category = determinedCategory
		}

		if err := repo.SaveArticle(ctx, &article); err != nil {
			return err
		}
	}

	return nil
}

func extractImageURLWithParser(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	var imageURL string
	var findImg func(*html.Node)
	findImg = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			// Found an img tag, look for src attribute
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					imageURL = attr.Val
					return
				}
			}
		}

		// Search in child nodes
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if imageURL != "" {
				return // Stop if we already found an image
			}
			findImg(c)
		}
	}

	findImg(doc)
	return imageURL
}

// Function to parse different time formats commonly used in RSS feeds
func parseRSSDate(dateStr string) (time.Time, error) {
	formats := []string{
		time.RFC1123Z, // "Mon, 02 Jan 2006 15:04:05 -0700"
		time.RFC1123,  // "Mon, 02 Jan 2006 15:04:05 MST"
		time.RFC822Z,  // "02 Jan 06 15:04 -0700"
		time.RFC822,   // "02 Jan 06 15:04 MST"
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
		"Mon, 02 Jan 2006 15:04:05",
		"2 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05 MST",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
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
func containsAny(content string, keywords []string) bool {
	contentWords := strings.Fields(content)

	// Создаем карту слов контента для быстрого поиска
	contentWordsMap := make(map[string]bool)
	for _, word := range contentWords {
		cleanedWord := strings.ToLower(word)
		cleanedWord = strings.Trim(cleanedWord, ".,!?:;\"'()[]{}")
		contentWordsMap[cleanedWord] = true
	}

	for _, keyword := range keywords {
		keywordLower := strings.ToLower(keyword)

		if strings.Contains(content, keywordLower) {
			return true
		}
		// Проверяем также, есть ли слово целиком в нашей карте слов
		if contentWordsMap[keywordLower] {
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

			err := processContent(context.Background(), logger, repo, task)
			if err != nil {
				logger.Error("error processing task: %v", err)
				d.Nack(false, false)
			} else {
				d.Ack(false)
				logger.Info("Task %s completed and acknowledged", task.ID)
			}
		}

		logger.Info("Message channel closed, exiting goroutine")
	}()

	logger.Info("Consumer started. Waiting for messages... Press Ctrl+C to exit.")
	<-forever
}
