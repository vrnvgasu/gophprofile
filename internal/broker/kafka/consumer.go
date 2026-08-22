package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/model"
)

type Handler func(ctx context.Context, event model.Event) error

type Consumer struct {
	client *kgo.Client
	tracer *kotel.Tracer
}

func NewConsumer(brokers []string, topic, groupID string) (*Consumer, error) {
	tracer := kotel.NewTracer(kotel.TracerProvider(otel.GetTracerProvider()))
	instr := kotel.NewKotel(kotel.WithTracer(tracer))

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topic),
		// Оффсет коммитится только после успешного возврата из обработчика.
		kgo.DisableAutoCommit(),
		kgo.AllowAutoTopicCreation(),
		kgo.WithHooks(instr.Hooks()...),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka.NewConsumer: %w", err)
	}

	return &Consumer{client: client, tracer: tracer}, nil
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
				logger.WithContext(ctx).Error("kafka.Run fetch", "topic", topic, "partition", partition, "error", err)
			}
		})

		var processed []*kgo.Record

		fetches.EachPartition(func(partition kgo.FetchTopicPartition) {
			processed = append(processed, processPartition(c.processSpan, handler, partition.Records)...)
		})

		if len(processed) == 0 {
			continue
		}

		if err := c.client.CommitRecords(ctx, processed...); err != nil && !errors.Is(err, context.Canceled) {
			logger.WithContext(ctx).Error("kafka.Run commit", "error", err)
		}
	}
}

type spanFunc func(*kgo.Record) (context.Context, trace.Span)

func (c *Consumer) processSpan(record *kgo.Record) (context.Context, trace.Span) {
	return c.tracer.WithProcessSpan(record)
}

// processPartition обрабатывает записи партиции по порядку и возвращает префикс, который можно коммитить.
func processPartition(newSpan spanFunc, handler Handler, records []*kgo.Record) []*kgo.Record {
	for i, record := range records {
		if err := handleRecord(newSpan, handler, record); err != nil {
			return records[:i]
		}
	}

	return records
}

func handleRecord(newSpan spanFunc, handler Handler, record *kgo.Record) error {
	ctx, span := newSpan(record)
	defer span.End()

	var event model.Event
	if err := json.Unmarshal(record.Value, &event); err != nil {
		logger.WithContext(ctx).Error("kafka.handleRecord Unmarshal",
			"offset", record.Offset, "error", err)
		span.SetStatus(codes.Error, "malformed event")

		return nil
	}

	span.SetAttributes(
		attribute.String("event.id", event.ID),
		attribute.String("event.type", string(event.Type)),
	)

	if err := handler(ctx, event); err != nil {
		logger.WithContext(ctx).Error("kafka.handleRecord handler",
			"event_id", event.ID, "type", event.Type, "offset", record.Offset, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return err
	}

	return nil
}

func (c *Consumer) Close() {
	c.client.Close()
}
