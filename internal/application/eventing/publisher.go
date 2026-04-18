package eventing

import (
	"context"

	domainevents "playground/internal/domain/events"
)

type Handler interface {
	Handle(ctx context.Context, event domainevents.Event) error
}

type HandlerFunc func(ctx context.Context, event domainevents.Event) error

func (f HandlerFunc) Handle(ctx context.Context, event domainevents.Event) error {
	return f(ctx, event)
}

type InProcessPublisher struct {
	handlers []Handler
}

func NewInProcessPublisher(handlers ...Handler) InProcessPublisher {
	items := make([]Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			items = append(items, handler)
		}
	}
	return InProcessPublisher{handlers: items}
}

func (p InProcessPublisher) Publish(ctx context.Context, events ...domainevents.Event) error {
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		if event == nil {
			continue
		}
		for _, handler := range p.handlers {
			if err := handler.Handle(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}
