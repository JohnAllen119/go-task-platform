package task

import (
	"encoding/json"
	"time"
)

type Task struct {
	ID           uint64
	TaskType     string
	Payload      json.RawMessage
	Status       string
	AttemptCount uint32
	MaxAttempts  uint32
	LastError    *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
}
