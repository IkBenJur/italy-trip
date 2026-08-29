package events

import (
	"net/http"
	"time"

	"github.com/IkBenJur/italy-trip/internal/json"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/utils"
	"github.com/gin-gonic/gin"
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
