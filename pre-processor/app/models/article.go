package models

import (
	"time"
)

type Article struct {
	ID        string    `db:"id"`
	Title     string    `db:"title"`
	Content   string    `db:"content"`
	URL       string    `db:"url"`
	CreatedAt time.Time `db:"created_at"`
}
