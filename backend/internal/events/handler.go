package events

import (
	"errors"
	"net/http"
	"time"

	"github.com/IkBenJur/italy-trip/internal/json"
	"github.com/IkBenJur/italy-trip/internal/middleware"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct {
	Queries repo.Querier
	Now     func() time.Time
}

func NewHandler(queries repo.Querier) *Handler {
	return &Handler{Queries: queries, Now: time.Now}
}

// eventResponse deliberately carries no photo URLs or storage keys — only the
// timing of the event and a count. A count is not content, and "42 photos
// waiting" is a nice thing to see on the camera screen.
type eventResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	IsOver     bool      `json:"is_over"`
	PhotoCount int64     `json:"photo_count"`
}

func (h *Handler) Current(c *gin.Context) {
	event, err := h.Queries.GetCurrentEvent(c)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			json.WriteErrorFromString(c, http.StatusNotFound, "no event has been started yet")
			return
		}
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to load event", err)
		return
	}

	count, err := h.Queries.CountPhotos(c, event.ID)
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to count photos", err)
		return
	}

	domain := FromRow(event)
	json.WriteJSON(c, http.StatusOK, eventResponse{
		ID:         utils.UUIDString(event.ID),
		Name:       domain.Name,
		StartsAt:   domain.StartsAt,
		EndsAt:     domain.EndsAt,
		IsOver:     domain.IsOver(h.Now()),
		PhotoCount: count,
	})
}

type createEventRequest struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

// Create starts a new event. The name is fixed (DefaultEventName) since the
// app only ever runs one named trip; only the timing is caller-supplied.
//
// Older events and their photos are left in place — this inserts a new row
// rather than replacing the current one, so past photos stay reachable under
// their own event.
func (h *Handler) Create(c *gin.Context) {
	user, ok := middleware.UserFromContext(c)
	if !ok {
		json.WriteErrorFromString(c, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if !req.EndsAt.After(req.StartsAt) {
		json.WriteErrorFromString(c, http.StatusBadRequest, "ends_at must be after starts_at")
		return
	}

	current, err := h.Queries.GetCurrentEvent(c)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to load current event", err)
		return
	}
	// A missing current event (pgx.ErrNoRows, the very first event) is fine —
	// there is nothing to conflict with.
	if err == nil && !FromRow(current).IsOver(h.Now()) {
		json.WriteErrorFromString(c, http.StatusConflict, "the current event has not ended yet")
		return
	}

	created, err := h.Queries.CreateEvent(c, repo.CreateEventParams{
		Name:     DefaultEventName,
		StartsAt: pgtype.Timestamptz{Time: req.StartsAt, Valid: true},
		EndsAt:   pgtype.Timestamptz{Time: req.EndsAt, Valid: true},
		UserID:   user.ID,
	})
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to create event", err)
		return
	}

	// Starting an event makes it the creator's active one.
	if err := h.Queries.SetActiveEvent(c, repo.SetActiveEventParams{
		ID:            user.ID,
		ActiveEventID: created.ID,
	}); err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to set active event", err)
		return
	}

	domain := FromRow(created)
	json.WriteJSON(c, http.StatusCreated, eventResponse{
		ID:         utils.UUIDString(created.ID),
		Name:       domain.Name,
		StartsAt:   domain.StartsAt,
		EndsAt:     domain.EndsAt,
		IsOver:     domain.IsOver(h.Now()),
		PhotoCount: 0,
	})
}
