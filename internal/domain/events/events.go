package events

import (
	"context"
	"time"
)

type Event interface {
	EventName() string
	OccurredAt() time.Time
}

type Publisher interface {
	Publish(ctx context.Context, events ...Event) error
}
