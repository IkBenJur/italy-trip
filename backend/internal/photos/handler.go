// Package photos owns the upload endpoint and the album endpoints. Every route
// here asks events.Event.IsOver before it does anything else, because these are
// the only routes that can move photo bytes or photo URLs across the wire.
package photos

import (
	"net/http"
	"time"

	"github.com/IkBenJur/italy-trip/internal/events"
	"github.com/IkBenJur/italy-trip/internal/json"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/storage"
	"github.com/gin-gonic/gin"
)

// DefaultMaxUploadBytes is 15 MB. A 1080p frame at quality 0.92 is 200-400 KB,
// so this is generous headroom rather than a real working limit.
const DefaultMaxUploadBytes int64 = 15 << 20

// PresignTTL is how long an album URL stays valid. Only ever minted after the
// event is over, so its lifetime is not a secrecy question.
const PresignTTL = time.Hour

type Handler struct {
	Queries        repo.Querier
	Storage        storage.Storage
	MaxUploadBytes int64

	// Now is injectable so tests can put the clock either side of ends_at.
	Now func() time.Time
}

func NewHandler(queries repo.Querier, store storage.Storage, maxUploadBytes int64) *Handler {
	if maxUploadBytes <= 0 {
		maxUploadBytes = DefaultMaxUploadBytes
	}
	return &Handler{
		Queries:        queries,
		Storage:        store,
		MaxUploadBytes: maxUploadBytes,
		Now:            time.Now,
	}
}

// ErrLocked is the message sent with 423. It says nothing about what is behind
// the lock.
const lockedMessage = "the event is not over yet"

// loadEvent fetches the singleton event row. It returns ok=false having already
// written an error response.
func (h *Handler) loadEvent(c *gin.Context) (repo.Event, bool) {
	row, err := h.Queries.GetCurrentEvent(c)
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to load event", err)
		return repo.Event{}, false
	}
	return row, true
}

// requireUnlocked is the album guard: it refuses with 423 while the event is
// still running. 423 (RFC 4918) is deliberately distinguishable from a 401/403,
// which is what lets the frontend route on it.
func (h *Handler) requireUnlocked(c *gin.Context) (repo.Event, bool) {
	row, ok := h.loadEvent(c)
	if !ok {
		return repo.Event{}, false
	}
	if !events.FromRow(row).IsOver(h.Now()) {
		json.WriteErrorFromString(c, http.StatusLocked, lockedMessage)
		return repo.Event{}, false
	}
	return row, true
}

// requireRunning is the upload guard: once the event is over, no more pictures.
func (h *Handler) requireRunning(c *gin.Context) (repo.Event, bool) {
	row, ok := h.loadEvent(c)
	if !ok {
		return repo.Event{}, false
	}
	if events.FromRow(row).IsOver(h.Now()) {
		json.WriteErrorFromString(c, http.StatusLocked, "the event is over; no more photos can be added")
		return repo.Event{}, false
	}
	return row, true
}
