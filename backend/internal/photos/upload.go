package photos

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/IkBenJur/italy-trip/internal/images"
	"github.com/IkBenJur/italy-trip/internal/json"
	"github.com/IkBenJur/italy-trip/internal/middleware"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/storage"
	"github.com/IkBenJur/italy-trip/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// uniqueViolation is Postgres' SQLSTATE for a unique constraint breach.
const uniqueViolation = "23505"

// uploadResponse carries an id and nothing else. No URL, no storage key: while
// the event runs, not even the uploader gets a way back to the bytes.
type uploadResponse struct {
	ID       string `json:"id"`
	ClientID string `json:"client_id"`
	// Duplicate marks a retry that the server had already accepted.
	Duplicate bool `json:"duplicate"`
}

// Upload handles POST /events/current/photos.
//
// The order of operations is load-bearing:
//  1. refuse outright once the event is over
//  2. cap the body before reading a byte of it
//  3. sniff the real content type; never trust the declared one
//  4. short-circuit a client_id we have already stored — this is what makes the
//     offline retry queue safe
//  5. only then decode, thumbnail, upload and insert
func (h *Handler) Upload(c *gin.Context) {
	event, ok := h.requireRunning(c)
	if !ok {
		return
	}

	user, ok := middleware.UserFromContext(c)
	if !ok {
		json.WriteErrorFromString(c, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Cap the body before anything reads it, so an oversized upload costs us the
	// limit and not the whole payload.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.MaxUploadBytes)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isTooLarge(err) {
			h.writeTooLarge(c)
			return
		}
		json.WriteError(c, http.StatusBadRequest, fmt.Errorf("missing or unreadable 'file' field: %w", err))
		return
	}

	clientID, err := parseClientID(c.PostForm("client_id"))
	if err != nil {
		json.WriteError(c, http.StatusBadRequest, err)
		return
	}

	takenAt, err := parseTakenAt(c.PostForm("taken_at"))
	if err != nil {
		json.WriteError(c, http.StatusBadRequest, err)
		return
	}

	// An already-stored client_id means this is a retry whose first response was
	// lost. Answer 200 and do no further work: no re-upload, no second row.
	existing, err := h.Queries.FindPhotoByClientId(c, repo.FindPhotoByClientIdParams{
		EventID:  event.ID,
		ClientID: clientID,
	})
	if err == nil {
		json.WriteJSON(c, http.StatusOK, uploadResponse{
			ID:        utils.UUIDString(existing.ID),
			ClientID:  existing.ClientID,
			Duplicate: true,
		})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to check for an existing photo", err)
		return
	}

	original, err := readUpload(fileHeader, h.MaxUploadBytes)
	if err != nil {
		if isTooLarge(err) {
			h.writeTooLarge(c)
			return
		}
		json.WriteError(c, http.StatusBadRequest, err)
		return
	}

	// Sniff the real type. A client can declare any Content-Type it likes.
	if detected := http.DetectContentType(original); detected != "image/jpeg" {
		json.WriteErrorFromString(c, http.StatusBadRequest,
			fmt.Sprintf("file must be a JPEG, detected %s", detected))
		return
	}

	thumb, err := images.Make(original)
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusBadRequest, "file is not a decodable JPEG", err)
		return
	}

	photoID := uuid.New()
	storageKey := storage.PhotoKey(photoID.String())
	thumbKey := storage.ThumbKey(photoID.String())

	if err := h.Storage.Put(c, storageKey, bytes.NewReader(original), "image/jpeg", int64(len(original))); err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusBadGateway, "failed to store photo", err)
		return
	}
	if err := h.Storage.Put(c, thumbKey, bytes.NewReader(thumb.JPEG), "image/jpeg", int64(len(thumb.JPEG))); err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusBadGateway, "failed to store thumbnail", err)
		return
	}

	created, err := h.Queries.CreatePhoto(c, repo.CreatePhotoParams{
		ID:          pgtype.UUID{Bytes: photoID, Valid: true},
		EventID:     event.ID,
		UploadedBy:  user.ID,
		ClientID:    clientID,
		StorageKey:  storageKey,
		ThumbKey:    thumbKey,
		ContentType: "image/jpeg",
		// Dimensions come from the decoded bitmap, not from the client.
		Width:     int32(thumb.Width),
		Height:    int32(thumb.Height),
		SizeBytes: int64(len(original)),
		TakenAt:   pgtype.Timestamptz{Time: takenAt, Valid: true},
	})
	if err != nil {
		// Two retries of the same capture can race past the lookup above. The
		// unique constraint catches the loser, and it is still a duplicate.
		if raced, ok := h.resolveRace(c, event.ID, clientID, err); ok {
			json.WriteJSON(c, http.StatusOK, uploadResponse{
				ID:        utils.UUIDString(raced.ID),
				ClientID:  raced.ClientID,
				Duplicate: true,
			})
			return
		}
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to save photo", err)
		return
	}

	json.WriteJSON(c, http.StatusCreated, uploadResponse{
		ID:       utils.UUIDString(created.ID),
		ClientID: created.ClientID,
	})
}

// resolveRace turns a unique-constraint violation back into the row that won.
func (h *Handler) resolveRace(c *gin.Context, eventID pgtype.UUID, clientID string, insertErr error) (repo.Photo, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(insertErr, &pgErr) || pgErr.Code != uniqueViolation {
		return repo.Photo{}, false
	}

	winner, err := h.Queries.FindPhotoByClientId(c, repo.FindPhotoByClientIdParams{
		EventID:  eventID,
		ClientID: clientID,
	})
	if err != nil {
		return repo.Photo{}, false
	}
	return winner, true
}

func (h *Handler) writeTooLarge(c *gin.Context) {
	json.WriteErrorFromString(c, http.StatusRequestEntityTooLarge,
		fmt.Sprintf("photo exceeds the %d byte limit", h.MaxUploadBytes))
}

// readUpload reads the uploaded part into memory. The body is already capped by
// MaxBytesReader; the extra LimitReader is belt and braces against a multipart
// part that claims a smaller size than it sends.
func readUpload(fileHeader *multipart.FileHeader, limit int64) ([]byte, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("open uploaded file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, &http.MaxBytesError{Limit: limit}
	}
	if len(data) == 0 {
		return nil, errors.New("uploaded file is empty")
	}
	return data, nil
}

func isTooLarge(err error) bool {
	var maxBytes *http.MaxBytesError
	return errors.As(err, &maxBytes)
}

func parseClientID(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("client_id is required")
	}
	// The client generates a UUID per capture and reuses it on every retry.
	// Requiring that shape here keeps junk out of the uniqueness key.
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("client_id must be a UUID: %w", err)
	}
	return parsed.String(), nil
}

func parseTakenAt(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, errors.New("taken_at is required")
	}
	// There is no EXIF on a canvas-captured frame, so capture time can only come
	// from the client saying so.
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("taken_at must be RFC3339: %w", err)
	}
	return parsed, nil
}
