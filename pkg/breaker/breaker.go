// Package breaker - предохранитель для вызовов внешних зависимостей.
package breaker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrOpen = errors.New("circuit is open")

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

type Config struct {
	FailureThreshold int
	OpenTimeout      time.Duration
	IsFailure        func(error) bool
}

func DefaultConfig() *Config {
	return &Config{
		FailureThreshold: 5,
		OpenTimeout:      10 * time.Second,
		IsFailure:        defaultIsFailure,
	}
}

func defaultIsFailure(err error) bool {
	return err != nil
}

type Breaker struct {
	name string
	cnf  *Config
	// now подменяется в тестах, чтобы не ждать реального тайм-аута.
	now func() time.Time

	mu       sync.Mutex
	state    State
	failures int
	openedAt time.Time
}

func NewWithSettings(name string, cnf *Config) *Breaker {
	if cnf == nil {
		cnf = DefaultConfig()
	}
	if cnf.IsFailure == nil {
		cnf.IsFailure = defaultIsFailure
	}

	return &Breaker{name: name, cnf: cnf, now: time.Now}
}

func New(name string) *Breaker {
	return NewWithSettings(name, DefaultConfig())
}

func (b *Breaker) Do(ctx context.Context, f func() error) error {
	if err := b.allow(); err != nil {
		return err
	}

	err := f()

	if ctx.Err() == nil {
		b.record(err)
	}

	return err
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.state
}

func (b *Breaker) allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateOpen:
		if b.now().Sub(b.openedAt) < b.cnf.OpenTimeout {
			return fmt.Errorf("breaker %s: %w", b.name, ErrOpen)
		}

		b.state = StateHalfOpen

		return nil
	case StateHalfOpen:
		return fmt.Errorf("breaker %s: %w", b.name, ErrOpen)
	default:
		return nil
	}
}

func (b *Breaker) record(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.cnf.IsFailure(err) {
		b.state = StateClosed
		b.failures = 0

		return
	}

	b.failures++

	if b.state == StateHalfOpen || b.failures >= b.cnf.FailureThreshold {
		b.state = StateOpen
		b.openedAt = b.now()
	}
}
