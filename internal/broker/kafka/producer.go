// Package kafka - работа с событиями об аватарках через Kafka.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel"

	"github.com/vrnvgasu/gophprofile/internal/metrics"
	"github.com/vrnvgasu/gophprofile/internal/model"
)

type Producer struct {
	client *kgo.Client
	topic  string
}

func NewProducer(brokers []string, topic string) (*Producer, error) {
	instr := kotel.NewKotel(kotel.WithTracer(
		kotel.NewTracer(kotel.TracerProvider(otel.GetTracerProvider())),
	))

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.AllowAutoTopicCreation(),
		kgo.WithHooks(instr.Hooks()...),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka.NewProducer: %w", err)
	}

	return &Producer{client: client, topic: topic}, nil
}

// Publish синхронно отправляет событие. Ключом сообщения служит идентификатор аватарки,
// поэтому все события одной аватарки попадают в одну партицию и обрабатываются по порядку.
func (p *Producer) Publish(ctx context.Context, key string, event model.Event) (err error) {
	defer func() {
		metrics.RecordEventPublished(ctx, string(event.Type), err)
	}()

	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("kafka.Publish Marshal: %w", err)
	}

	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(key),
		Value: value,
	}

	if err = p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("kafka.Publish ProduceSync: %w", err)
	}

	return nil
}

func (p *Producer) Ping(ctx context.Context) error {
	if err := p.client.Ping(ctx); err != nil {
		return fmt.Errorf("kafka.Ping: %w", err)
	}

	return nil
}

func (p *Producer) Close() {
	p.client.Close()
}
