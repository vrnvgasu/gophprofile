package kafka

import (
	"context"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/metrics"
)

// WatchLag периодически публикует лаг consumer-группы в метрики.
func (c *Consumer) WatchLag(ctx context.Context, group string, interval time.Duration) {
	admin := kadm.NewClient(c.client)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.reportLag(ctx, admin, group)
		}
	}
}

func (c *Consumer) reportLag(ctx context.Context, admin *kadm.Client, group string) {
	lags, err := admin.Lag(ctx, group)
	if err != nil {
		logger.WithContext(ctx).Error("kafka.WatchLag", "error", err)

		return
	}

	for _, groupLag := range lags {
		for _, l := range groupLag.Lag.Sorted() {
			if l.Lag < 0 {
				continue
			}

			metrics.RecordConsumerLag(ctx, l.Topic, l.Partition, l.Lag)
		}
	}
}
