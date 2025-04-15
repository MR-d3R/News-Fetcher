package models

import (
	"time"
)

type FetchTask struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	SourceURL string    `json:"source_url"`
	CreatedAt time.Time `json:"created_at"`
}
