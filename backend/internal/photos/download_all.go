package photos

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/IkBenJur/italy-trip/internal/json"
	"github.com/IkBenJur/italy-trip/internal/storage"
	"github.com/gin-gonic/gin"
)

// DownloadAll handles GET /events/current/photos/download: every photo
// belonging to the current event, streamed as one zip file so the browser
// makes a single authenticated request instead of one per photo, 300ms apart.
//
// Mirrors Original's streaming approach (handler.go's comment on that route):
// the archive is written directly to c.Writer as each entry is read from
// storage, rather than buffered fully in memory first.
func (h *Handler) DownloadAll(c *gin.Context) {
	event, ok := h.requireUnlocked(c)
	if !ok {
		return
	}

	rows, err := h.Queries.ListPhotosByEvent(c, event.ID)
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to list photos", err)
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", storage.ContentDisposition(zipFilename(event.Name)))
	c.Status(http.StatusOK)

	zw := zip.NewWriter(c.Writer)
	defer func() {
		if err := zw.Close(); err != nil {
			slog.Error("failed to finalize zip", "error", err)
		}
	}()

	names := make(map[string]int) // dedupe same-second bursts
	for _, row := range rows {
		object, err := h.Storage.Open(c, row.StorageKey)
		if err != nil {
			// The 200 status and zip headers are already on the wire, so this
			// can't turn into a clean HTTP error any more. Stopping here (the
			// deferred zw.Close still runs) hands the client a shorter-than-
			// expected but structurally valid zip — missing whatever photo
			// failed to open and everything after it — rather than hanging
			// or silently completing over a gap.
			slog.Error("failed to open photo for zip", "key", row.StorageKey, "error", err)
			return
		}

		entry, err := zw.Create(uniqueName(names, downloadFilename(row.TakenAt.Time)))
		if err == nil {
			_, err = io.Copy(entry, object.Body)
		}
		object.Body.Close()
		if err != nil {
			slog.Error("failed to write photo into zip", "key", row.StorageKey, "error", err)
			return
		}
	}
}

// zipFilename names the archive after the event, e.g. "Italy Trip" becomes
// "italy-trip-photos.zip". DefaultEventName never actually varies today, but
// events are timestamped rows rather than a fixed singleton (see
// internal/events), so this doesn't hardcode the one name that happens to
// exist right now.
func zipFilename(eventName string) string {
	return slugify(eventName) + "-photos.zip"
}

func slugify(name string) string {
	var b strings.Builder
	lastWasDash := true // swallow any leading separator instead of starting with "-"
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastWasDash = false
		case !lastWasDash:
			b.WriteByte('-')
			lastWasDash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "photos"
	}
	return out
}

// uniqueName gives a photo landing on the same downloadFilename as an earlier
// one a distinct zip entry name instead of silently overwriting it.
// downloadFilename only has second resolution, and a burst of captures can
// land in the same second.
func uniqueName(seen map[string]int, name string) string {
	seen[name]++
	if seen[name] == 1 {
		return name
	}

	ext := ""
	base := name
	if idx := strings.LastIndex(name, "."); idx != -1 {
		ext = name[idx:]
		base = name[:idx]
	}
	return fmt.Sprintf("%s-%d%s", base, seen[name], ext)
}
