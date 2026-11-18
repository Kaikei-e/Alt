package domain

import (
	"time"

	"github.com/google/uuid"
)

const QueuePolicyVersionV1 = 1

type QueueArticleOutcome string

const (
	QueueArticleOutcomeQueued        QueueArticleOutcome = "queued"
	QueueArticleOutcomeAlreadyQueued QueueArticleOutcome = "already_queued"
	QueueArticleOutcomeQueueFull     QueueArticleOutcome = "queue_full"
)

type ReadingQueueEntry struct {
	ArticleID          uuid.UUID
	QueuedAt           time.Time
	ExpiresAt          time.Time
	QueuePolicyVersion int
}

type HydratedReadingQueueItem struct {
	ArticleID  uuid.UUID
	Title      string
	URL        string
	SourceName string
	ImageURL   string
	QueuedAt   time.Time
	ExpiresAt  time.Time
}
