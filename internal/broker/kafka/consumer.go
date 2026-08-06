package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/model"
)

type Handler func(ctx context.Context, event model.Event) error

type Consumer struct {
	client *kgo.Client
}

func NewConsumer(brokers []string, topic, groupID string) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topic),
		// Оффсет коммитится только после успешного возврата из обработчика.
		kgo.DisableAutoCommit(),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka.NewConsumer: %w", err)
	}

	return &Consumer{client: client}, nil
}

func (c *Consumer) Run(ctx context.Context, handler Handler) {
	for {
		if ctx.Err() != nil {
			return
		}

		fetches := c.client.PollFetches(ctx)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			return
		}

		fetches.EachError(func(topic string, partition int32, err error) {
			if !errors.Is(err, context.Canceled) {
				logger.Log.Errorw("kafka.Run fetch", "topic", topic, "partition", partition, "error", err)
			}
		})

		fetches.EachRecord(func(record *kgo.Record) {
			c.handleRecord(ctx, handler, record)
		})

		if err := c.client.CommitUncommittedOffsets(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Log.Errorw("kafka.Run commit", "error", err)
		}
	}
}

func (c *Consumer) handleRecord(ctx context.Context, handler Handler, record *kgo.Record) {
	var event model.Event
	if err := json.Unmarshal(record.Value, &event); err != nil {
		// Битое сообщение не повторяем.
		logger.Log.Errorw("kafka.handleRecord Unmarshal",
			"offset", record.Offset, "error", err)
		return
	}

	if err := handler(ctx, event); err != nil {
		logger.Log.Errorw("kafka.handleRecord handler",
			"event_id", event.ID, "type", event.Type, "error", err)
	}
}

func (c *Consumer) Close() {
	c.client.Close()
}
