package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Article struct {
	ID          int       `json:"id"`
	Author      string    `json:"author"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	URL         string    `json:"url"`
	ImageURL    string    `json:"urlToImage"`
	Category    string    `json:"category"`
	PublishedAt time.Time `json:"publishedAt"`
}

type ArticleRepository struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewArticleRepository(db *pgxpool.Pool, redis *redis.Client) *ArticleRepository {
	return &ArticleRepository{
		db:    db,
		redis: redis,
	}
}

// GetArticleByID пытается получить статью из кэша, если не находит - берет из БД
func (r *ArticleRepository) GetArticleByID(ctx context.Context, id int) (*Article, error) {
	// Пробуем получить из кэша
	cacheKey := fmt.Sprintf("article:%d", id)
	cachedData, err := r.redis.Get(ctx, cacheKey).Result()

	// Если данные есть в кэше и нет ошибки
	if err == nil {
		var article Article
		if err := json.Unmarshal([]byte(cachedData), &article); err == nil {
			return &article, nil
		}
	}

	// Если не нашли в кэше или произошла ошибка при десериализации,
	// запрашиваем из PostgreSQL
	var article Article
	err = r.db.QueryRow(ctx,
		"SELECT id, title, description, content, url, image_url, category, publishedAt FROM articles WHERE id = $1",
		id).Scan(&article.ID, &article.Title, &article.Description, &article.Content, &article.URL, &article.ImageURL, &article.Category, &article.PublishedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to get article: %w", err)
	}

	// Сохраняем в кэш на определенное время
	articleJSON, _ := json.Marshal(article)
	r.redis.Set(ctx, cacheKey, articleJSON, time.Minute*15)

	return &article, nil
}

// SaveArticle сохраняет статью в БД и обновляет кэш
func (r *ArticleRepository) SaveArticle(ctx context.Context, article *Article) error {
	var id int
	err := r.db.QueryRow(ctx,
		"INSERT INTO articles(title, description, content, url, image_url, category, publishedAt) VALUES($1, $2, $3, $4, $5, $6, $7) RETURNING id",
		article.Title, article.Description, article.Content, article.URL, article.ImageURL, article.Category, article.PublishedAt).Scan(&id)

	if err != nil {
		return fmt.Errorf("failed to save article: %w", err)
	}

	article.ID = id

	// Обновляем кэш
	cacheKey := fmt.Sprintf("article:%d", article.ID)
	articleJSON, _ := json.Marshal(article)
	r.redis.Set(ctx, cacheKey, articleJSON, time.Minute*15)

	// Инвалидируем кэш для списков, которые могут содержать эту статью
	r.redis.Del(ctx, "recent:articles", fmt.Sprintf("category:%s:articles", article.Category))

	return nil
}

// GetRecentArticles получает недавние статьи с использованием кэша
func (r *ArticleRepository) GetRecentArticles(ctx context.Context, limit int) ([]*Article, error) {
	// Пробуем получить из кэша
	cacheKey := "recent:articles"
	cachedData, err := r.redis.Get(ctx, cacheKey).Result()

	if err == nil {
		var articles []*Article
		if err := json.Unmarshal([]byte(cachedData), &articles); err == nil {
			return articles, nil
		}
	}

	// Если не нашли в кэше, запрашиваем из PostgreSQL
	rows, err := r.db.Query(ctx,
		"SELECT id, title, description, content, url, image_url, category, publishedAt FROM articles ORDER BY publishedAt DESC LIMIT $1",
		limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent articles: %w", err)
	}
	defer rows.Close()

	var articles []*Article
	for rows.Next() {
		var article Article
		if err := rows.Scan(&article.ID, &article.Title, &article.Description, &article.Content, &article.URL, &article.ImageURL, &article.Category, &article.PublishedAt); err != nil {
			return nil, err
		}
		articles = append(articles, &article)
	}

	// Сохраняем в кэш
	articlesJSON, _ := json.Marshal(articles)
	r.redis.Set(ctx, cacheKey, articlesJSON, time.Minute*5) // меньше TTL для списков

	return articles, nil
}
