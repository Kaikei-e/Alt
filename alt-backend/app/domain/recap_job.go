package domain

import (
	"time"
)

type RecapJob struct {
	JobID      string     `json:"job_id"`
	Status     string     `json:"status"`
	LastStage  *string    `json:"last_stage"`
	KickedAt   time.Time  `json:"kicked_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
