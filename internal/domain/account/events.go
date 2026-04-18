package account

import (
	"time"

	domainevents "playground/internal/domain/events"
)

type ProfileUpdated struct {
	AccountID string
	TenantID  string
	At        time.Time
}

func (e ProfileUpdated) EventName() string {
	return "account.profile_updated"
}

func (e ProfileUpdated) OccurredAt() time.Time {
	return e.At
}

type PasswordChanged struct {
	AccountID string
	TenantID  string
	At        time.Time
}

func (e PasswordChanged) EventName() string {
	return "account.password_changed"
}

func (e PasswordChanged) OccurredAt() time.Time {
	return e.At
}

type Deleted struct {
	AccountID string
	TenantID  string
	At        time.Time
}

func (e Deleted) EventName() string {
	return "account.deleted"
}

func (e Deleted) OccurredAt() time.Time {
	return e.At
}

func (a *Account) recordEvent(event domainevents.Event) {
	a.events = append(a.events, event)
}

func (a *Account) PullEvents() []domainevents.Event {
	if len(a.events) == 0 {
		return nil
	}

	events := make([]domainevents.Event, len(a.events))
	copy(events, a.events)
	a.events = nil
	return events
}

func (a *Account) MarkDeleted(now time.Time) {
	a.recordEvent(Deleted{
		AccountID: a.ID,
		TenantID:  a.TenantID,
		At:        now.UTC(),
	})
}
