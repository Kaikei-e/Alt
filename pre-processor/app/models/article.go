package models

import (
	"time"
)

type Article struct {
	CreatedAt time.Time `db:"created_at"`
	ID        string    `db:"id"`
	Title     string    `db:"title"`
	Content   string    `db:"content"`
	URL       string    `db:"url"`
}
