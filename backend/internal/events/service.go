package events

import (
	"time"

	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/utils"
)

// Event is the domain view of the singleton event row, with real time.Time
// values rather than pgtype wrappers.
type Event struct {
	ID       string
	Name     string
	StartsAt time.Time
	EndsAt   time.Time
}

// IsOver is the single source of truth for the lock. Every handler that could
// emit photo bytes or a photo URL asks this one question, so there is exactly
// one place the rule can be wrong.
//
// The boundary is inclusive: at exactly EndsAt the event is over and the album
// opens.
func (e Event) IsOver(now time.Time) bool {
	return !now.Before(e.EndsAt)
}

// FromRow converts a database row into the domain type.
func FromRow(row repo.Event) Event {
	return Event{
		ID:       utils.UUIDString(row.ID),
		Name:     row.Name,
		StartsAt: row.StartsAt.Time,
		EndsAt:   row.EndsAt.Time,
	}
}
