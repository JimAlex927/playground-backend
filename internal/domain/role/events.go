package role

import (
	"time"

	domainevents "playground/internal/domain/events"
)

type Updated struct {
	RoleID   string
	TenantID string
	Name     string
	Occurred time.Time
}

func (e Updated) EventName() string {
	return "role.updated"
}

func (e Updated) OccurredAt() time.Time {
	return e.Occurred
}

func (r *Role) recordEvent(event domainevents.Event) {
	r.events = append(r.events, event)
}

func (r *Role) PullEvents() []domainevents.Event {
	if len(r.events) == 0 {
		return nil
	}

	events := make([]domainevents.Event, len(r.events))
	copy(events, r.events)
	r.events = nil
	return events
}
