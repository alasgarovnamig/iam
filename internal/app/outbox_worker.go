package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

// EventSink is the Kafka publisher port used by the outbox worker.
type EventSink interface {
	Publish(ctx context.Context, ev *domain.OutboxEvent) error
	PublishDead(ctx context.Context, dlqTopic string, ev *domain.OutboxEvent, lastError string) error
}

// OutboxWorker drains the transactional outbox to Kafka with retries,
// exponential backoff and dead-lettering. Business transactions never touch
// Kafka directly: if Kafka is down, events wait in the outbox.
type OutboxWorker struct {
	Tx           ports.TxManager
	Outbox       ports.OutboxRepo
	Sink         EventSink
	Logger       *slog.Logger
	PollInterval time.Duration
	BatchSize    int
	MaxAttempts  int
	DLQTopic     string
	Now          func() time.Time

	// hooks for metrics (optional)
	OnPublished func()
	OnFailed    func()
}

func (w *OutboxWorker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now().UTC()
}

// Run loops until ctx is cancelled.
func (w *OutboxWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()
	w.Logger.Info("outbox worker started", "poll_interval", w.PollInterval, "batch_size", w.BatchSize)
	for {
		select {
		case <-ctx.Done():
			w.Logger.Info("outbox worker stopping")
			return
		case <-ticker.C:
			if n, err := w.Drain(ctx); err != nil {
				w.Logger.Error("outbox drain failed", "error", err)
			} else if n > 0 {
				w.Logger.Debug("outbox drained", "published", n)
			}
		}
	}
}

// Drain processes one batch; returns the number of successfully published events.
func (w *OutboxWorker) Drain(ctx context.Context) (int, error) {
	published := 0
	err := w.Tx.WithTx(ctx, ports.DBContext{Platform: true}, func(txCtx context.Context) error {
		batch, err := w.Outbox.FetchBatch(txCtx, w.BatchSize, w.now())
		if err != nil {
			return err
		}
		for _, ev := range batch {
			if err := w.Sink.Publish(ctx, ev); err != nil {
				attempt := ev.Attempts + 1
				if attempt >= w.MaxAttempts {
					// Try dead-letter; if even that fails, keep as failed.
					if dlqErr := w.Sink.PublishDead(ctx, w.DLQTopic, ev, err.Error()); dlqErr == nil {
						if mErr := w.Outbox.MarkDead(txCtx, ev.ID, err.Error()); mErr != nil {
							return mErr
						}
						w.Logger.Warn("outbox event dead-lettered", "event_id", ev.EventID, "error", err)
						continue
					}
				}
				next := w.now().Add(domain.BackoffDelay(attempt))
				if mErr := w.Outbox.MarkFailed(txCtx, ev.ID, attempt, next, err.Error()); mErr != nil {
					return mErr
				}
				if w.OnFailed != nil {
					w.OnFailed()
				}
				w.Logger.Warn("outbox publish failed, will retry", "event_id", ev.EventID,
					"attempt", attempt, "next_attempt_at", next, "error", err)
				continue
			}
			if err := w.Outbox.MarkPublished(txCtx, ev.ID, w.now()); err != nil {
				return err
			}
			if w.OnPublished != nil {
				w.OnPublished()
			}
			published++
		}
		return nil
	})
	return published, err
}
