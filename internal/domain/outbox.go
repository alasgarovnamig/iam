package domain

import "time"

type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "pending"
	OutboxPublished OutboxStatus = "published"
	OutboxFailed    OutboxStatus = "failed"
	OutboxDead      OutboxStatus = "dead"
)

// OutboxEvent is a domain event persisted in the same transaction as the
// state change, later published to Kafka by the outbox worker.
type OutboxEvent struct {
	ID            int64
	EventID       string
	EventType     string
	EventVersion  int
	AggregateType string
	AggregateID   string
	TenantID      string
	Payload       []byte
	Headers       map[string]string
	Status        OutboxStatus
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

// BackoffDelay returns the exponential retry delay for the given attempt.
func BackoffDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	return time.Duration(1<<uint(attempt-1)) * time.Second
}
