package avatar

import (
	"context"
	"time"
)

const componentTimeout = 2 * time.Second

const (
	StatusUp   = "up"
	StatusDown = "down"
)

type Health struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components"`
}

func (s *Service) Check(ctx context.Context) Health {
	health := Health{
		Status: StatusUp,
		Components: map[string]string{
			"database": s.checkComponent(ctx, s.storage.Ping),
			"s3":       s.checkComponent(ctx, s.objects.Ping),
			"broker":   s.checkComponent(ctx, s.producer.Ping),
		},
	}

	for _, status := range health.Components {
		if status != StatusUp {
			health.Status = StatusDown
			break
		}
	}

	return health
}

func (s *Service) checkComponent(ctx context.Context, ping func(context.Context) error) string {
	ctx, cancel := context.WithTimeout(ctx, componentTimeout)
	defer cancel()

	if err := ping(ctx); err != nil {
		return StatusDown
	}

	return StatusUp
}
