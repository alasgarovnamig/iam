// Package kafka implements the event publisher used by the outbox worker.
package kafka

import (
	"context"
	"fmt"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/sr-alasgarov/wallet-gateway/internal/domain"
)

// Publisher writes outbox events to Kafka. Topic = <prefix>.<aggregate_type>,
// key = aggregate id (per-aggregate ordering).
type Publisher struct {
	writer      *kafkago.Writer
	topicPrefix string
}

func NewPublisher(brokers []string, topicPrefix string) *Publisher {
	return &Publisher{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(brokers...),
			Balancer:               &kafkago.Hash{},
			RequiredAcks:           kafkago.RequireAll,
			AllowAutoTopicCreation: true,
			BatchTimeout:           50 * time.Millisecond,
		},
		topicPrefix: topicPrefix,
	}
}

func (p *Publisher) TopicFor(ev *domain.OutboxEvent) string {
	return fmt.Sprintf("%s.%s", p.topicPrefix, strings.ReplaceAll(ev.AggregateType, "_", "-"))
}

func (p *Publisher) Publish(ctx context.Context, ev *domain.OutboxEvent) error {
	headers := []kafkago.Header{
		{Key: "event_id", Value: []byte(ev.EventID)},
		{Key: "event_type", Value: []byte(ev.EventType)},
	}
	for k, v := range ev.Headers {
		headers = append(headers, kafkago.Header{Key: k, Value: []byte(v)})
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Topic:   p.TopicFor(ev),
		Key:     []byte(ev.AggregateID),
		Value:   ev.Payload,
		Headers: headers,
	})
}

// PublishDead sends an exhausted event to the dead-letter topic.
func (p *Publisher) PublishDead(ctx context.Context, dlqTopic string, ev *domain.OutboxEvent, lastError string) error {
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Topic: dlqTopic,
		Key:   []byte(ev.AggregateID),
		Value: ev.Payload,
		Headers: []kafkago.Header{
			{Key: "event_id", Value: []byte(ev.EventID)},
			{Key: "event_type", Value: []byte(ev.EventType)},
			{Key: "original_topic", Value: []byte(p.TopicFor(ev))},
			{Key: "last_error", Value: []byte(lastError)},
		},
	})
}

func (p *Publisher) Close() error { return p.writer.Close() }
