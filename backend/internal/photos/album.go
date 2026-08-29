package photos

import (
	"errors"
	"net/http"
	"time"

	"github.com/IkBenJur/italy-trip/internal/json"
	"github.com/IkBenJur/italy-trip/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// photoResponse is the album view of a photo. It only ever exists on the far
// side of the lock — requireUnlocked runs before any of these are built.
type photoResponse struct {
	ID       string    `json:"id"`
	TakenAt  time.Time `json:"taken_at"`
	Width    int32     `json:"width"`
	Height   int32     `json:"height"`
	URL      string    `json:"url"`
	ThumbURL string    `json:"thumb_url"`
}

type listResponse struct {
	Photos []photoResponse `json:"photos"`
}

// List handles GET /events/current/photos.
//
// While the event runs this is a 423 with an error string and nothing else: no
// ids, no counts, no keys, and above all no signed URLs. A client with devtools
// open and a wrong system clock gets the same 423.
func (h *Handler) List(c *gin.Context) {
	event, ok := h.requireUnlocked(c)
	if !ok {
		return
	}

	rows, err := h.Queries.ListPhotosByEvent(c, event.ID)
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to list photos", err)
		return
	}

	out := make([]photoResponse, 0, len(rows))
	for _, row := range rows {
		url, err := h.Storage.PresignGet(c, row.StorageKey, PresignTTL)
		if err != nil {
			json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusBadGateway, "failed to sign photo url", err)
			return
		}
		thumbURL, err := h.Storage.PresignGet(c, row.ThumbKey, PresignTTL)
		if err != nil {
			json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusBadGateway, "failed to sign thumbnail url", err)
			return
		}

		out = append(out, photoResponse{
			ID:       utils.UUIDString(row.ID),
			TakenAt:  row.TakenAt.Time,
			Width:    row.Width,
			Height:   row.Height,
			URL:      url,
			ThumbURL: thumbURL,
		})
	}

	json.WriteJSON(c, http.StatusOK, listResponse{Photos: out})
}

// Original handles GET /photos/:id/original: a 302 to a presigned URL carrying
// an attachment disposition, which is what makes a phone save the file to the
// camera roll rather than opening it in a tab.
func (h *Handler) Original(c *gin.Context) {
	if _, ok := h.requireUnlocked(c); !ok {
		return
	}

	id, err := utils.ParseUUID(c.Param("id"))
	if err != nil {
		json.WriteErrorFromString(c, http.StatusBadRequest, "photo id must be a UUID")
		return
	}

	row, err := h.Queries.FindPhotoById(c, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			json.WriteErrorFromString(c, http.StatusNotFound, "photo not found")
			return
		}
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to load photo", err)
		return
	}

	signed, err := h.Storage.PresignDownload(c, row.StorageKey, downloadFilename(row.TakenAt.Time), PresignTTL)
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusBadGateway, "failed to sign download url", err)
		return
	}

	c.Redirect(http.StatusFound, signed)
}

// downloadFilename names the saved file after when it was taken, so a camera
// roll full of these sorts sensibly.
func downloadFilename(takenAt time.Time) string {
	return "italy-" + takenAt.UTC().Format("20060102-150405") + ".jpg"
}
