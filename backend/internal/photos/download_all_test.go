package photos

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// seedPhotoRowOnly inserts a photo row without storing its bytes, simulating
// storage/database drift so Storage.Open fails for just this one photo.
func (h *harness) seedPhotoRowOnly(t *testing.T, takenAt time.Time) repo.Photo {
	t.Helper()

	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	idStr := uuidString(id)

	photo, err := h.queries.CreatePhoto(t.Context(), repo.CreatePhotoParams{
		ID:          id,
		EventID:     h.eventID,
		UploadedBy:  h.userID,
		ClientID:    uuid.NewString(),
		StorageKey:  storage.PhotoKey(idStr),
		ThumbKey:    storage.ThumbKey(idStr),
		ContentType: "image/jpeg",
		Width:       1920,
		Height:      1080,
		SizeBytes:   300000,
		TakenAt:     pgtype.Timestamptz{Time: takenAt, Valid: true},
	})
	if err != nil {
		t.Fatalf("seed photo row: %v", err)
	}
	return photo
}

func readZip(t *testing.T, res *httptest.ResponseRecorder) *zip.Reader {
	t.Helper()
	body := res.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("response is not a valid zip: %v; body: %v", err, body)
	}
	return zr
}

func TestDownloadAllIsLockedDuringTheEvent(t *testing.T) {
	h := newHarness(t, duringEvent)
	photo := h.seedPhoto(t, duringEvent.Add(-time.Hour))

	res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos/download", nil), true)
	body := readBody(t, res)

	if res.Code != http.StatusLocked {
		t.Fatalf("status = %d, want 423; body: %s", res.Code, body)
	}
	assertNoLeak(t, body)
	assertNoRawLeak(t, res, photo)
	if disposition := res.Header().Get("Content-Disposition"); disposition != "" {
		t.Fatalf("locked response carries a Content-Disposition header: %s", disposition)
	}
	if ct := res.Header().Get("Content-Type"); strings.Contains(ct, "zip") {
		t.Fatalf("locked response advertises a zip Content-Type: %s", ct)
	}
}

func TestDownloadAllHappyPath(t *testing.T) {
	h := newHarness(t, afterEvent)

	first := h.seedPhoto(t, time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC))
	second := h.seedPhoto(t, time.Date(2026, 9, 8, 18, 30, 0, 0, time.UTC))
	third := h.seedPhoto(t, time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC))

	res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos/download", nil), true)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", res.Code, readBody(t, res))
	}
	if ct := res.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if disposition := res.Header().Get("Content-Disposition"); !strings.Contains(disposition, "italy-trip-photos.zip") {
		t.Errorf("Content-Disposition = %q, want it to name italy-trip-photos.zip", disposition)
	}

	zr := readZip(t, res)
	if len(zr.File) != 3 {
		t.Fatalf("got %d zip entries, want 3", len(zr.File))
	}

	want := map[string]bool{
		downloadFilename(first.TakenAt.Time):  true,
		downloadFilename(second.TakenAt.Time): true,
		downloadFilename(third.TakenAt.Time):  true,
	}
	for _, f := range zr.File {
		if !want[f.Name] {
			t.Errorf("unexpected zip entry %q", f.Name)
		}
		delete(want, f.Name)
	}
	if len(want) != 0 {
		t.Errorf("zip is missing entries: %v", want)
	}
}

func TestDownloadAllDedupesSameSecondCollisions(t *testing.T) {
	h := newHarness(t, afterEvent)

	takenAt := time.Date(2026, 9, 6, 12, 30, 45, 0, time.UTC)
	h.seedPhoto(t, takenAt)
	h.seedPhoto(t, takenAt)

	res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos/download", nil), true)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", res.Code, readBody(t, res))
	}

	zr := readZip(t, res)
	if len(zr.File) != 2 {
		t.Fatalf("got %d zip entries, want 2", len(zr.File))
	}

	names := map[string]bool{}
	for _, f := range zr.File {
		if names[f.Name] {
			t.Fatalf("duplicate zip entry name %q", f.Name)
		}
		names[f.Name] = true
	}

	base := downloadFilename(takenAt)
	if !names[base] {
		t.Errorf("expected an entry named %q, got %v", base, names)
	}
	if !names[strings.TrimSuffix(base, ".jpg")+"-2.jpg"] {
		t.Errorf("expected the collision to be suffixed, got %v", names)
	}
}

func TestDownloadAllReturnsEmptyZipForNoPhotos(t *testing.T) {
	h := newHarness(t, afterEvent)

	res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos/download", nil), true)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", res.Code, readBody(t, res))
	}

	zr := readZip(t, res)
	if len(zr.File) != 0 {
		t.Fatalf("got %d zip entries, want 0 for an empty event", len(zr.File))
	}
}

func TestDownloadAllStopsCleanlyWhenStorageFailsMidStream(t *testing.T) {
	h := newHarness(t, afterEvent)
	h.seedPhoto(t, time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC))
	// The row exists but its bytes were never stored — simulates storage/DB
	// drift, so Storage.Open fails for just this one photo, after the first
	// has already been written into the archive.
	h.seedPhotoRowOnly(t, time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC))

	res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos/download", nil), true)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", res.Code, readBody(t, res))
	}

	// The deferred zw.Close still runs on the early return, so the client
	// gets a structurally valid zip — just missing the photo that failed to
	// open (and anything after it), rather than a hang or a panic.
	zr := readZip(t, res)
	if len(zr.File) != 1 {
		t.Fatalf("got %d zip entries, want exactly 1 (the photo that opened before the failure)", len(zr.File))
	}
}
