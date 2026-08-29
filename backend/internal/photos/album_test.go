package photos

import (
	"context"
	"encoding/json"
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

// seedPhoto puts a row and its two objects in place directly, so album tests do
// not have to go through the upload path.
func (h *harness) seedPhoto(t *testing.T, takenAt time.Time) repo.Photo {
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
		t.Fatalf("seed photo: %v", err)
	}

	body := jpegBytes(t, 320, 180)
	h.store.Put(t.Context(), photo.StorageKey, strings.NewReader(string(body)), "image/jpeg", int64(len(body)))
	h.store.Put(t.Context(), photo.ThumbKey, strings.NewReader(string(body)), "image/jpeg", int64(len(body)))

	return photo
}

// TestAlbumIsLockedDuringTheEvent is the test the whole product rests on. A
// locked response must contain no storage key, no bucket name and nothing that
// looks like a signature — not merely a 423 status the client could ignore.
func TestAlbumIsLockedDuringTheEvent(t *testing.T) {
	h := newHarness(t, duringEvent)
	photo := h.seedPhoto(t, duringEvent.Add(-time.Hour))

	t.Run("GET /events/current/photos", func(t *testing.T) {
		res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos", nil), true)
		body := readBody(t, res)

		if res.Code != http.StatusLocked {
			t.Fatalf("status = %d, want 423; body: %s", res.Code, body)
		}
		assertNoLeak(t, body)
		assertNoRawLeak(t, res, photo)
	})

	t.Run("GET /photos/:id/original", func(t *testing.T) {
		res := h.do(httptest.NewRequest(http.MethodGet, "/photos/"+uuidString(photo.ID)+"/original", nil), true)
		body := readBody(t, res)

		if res.Code != http.StatusLocked {
			t.Fatalf("status = %d, want 423; body: %s", res.Code, body)
		}
		// A 302 would leak the URL in a header rather than the body, so the
		// Location header has to be empty too.
		if location := res.Header().Get("Location"); location != "" {
			t.Fatalf("locked response carries a Location header: %s", location)
		}
		assertNoLeak(t, body)
		assertNoRawLeak(t, res, photo)
	})

	t.Run("a photo id that does not exist is still just locked", func(t *testing.T) {
		// Answering 404 here would confirm which ids exist; 423 says nothing.
		res := h.do(httptest.NewRequest(http.MethodGet, "/photos/"+uuid.NewString()+"/original", nil), true)
		if res.Code != http.StatusLocked {
			t.Fatalf("status = %d, want 423", res.Code)
		}
	})

	t.Run("423 is distinguishable from an auth failure", func(t *testing.T) {
		res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos", nil), false)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated status = %d, want 401 (not 423)", res.Code)
		}
	})
}

// assertNoRawLeak greps the entire raw response — headers included — for this
// photo's actual keys and for signature parameters.
func assertNoRawLeak(t *testing.T, res *httptest.ResponseRecorder, photo repo.Photo) {
	t.Helper()

	var raw strings.Builder
	res.Header().Write(&raw)
	raw.WriteString(res.Body.String())
	haystack := raw.String()

	for _, needle := range []string{
		photo.StorageKey,
		photo.ThumbKey,
		"X-Amz-Signature",
		"X-Amz-Credential",
		"fake-bucket.example.com",
	} {
		if strings.Contains(haystack, needle) {
			t.Fatalf("raw response leaked %q; full response: %s", needle, haystack)
		}
	}
}

func TestAlbumOpensOnceTheEventIsOver(t *testing.T) {
	h := newHarness(t, afterEvent)

	// Seeded out of chronological order on purpose: with a retry queue, upload
	// order and capture order genuinely differ.
	third := h.seedPhoto(t, time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC))
	first := h.seedPhoto(t, time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC))
	second := h.seedPhoto(t, time.Date(2026, 9, 8, 18, 30, 0, 0, time.UTC))

	res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos", nil), true)
	body := readBody(t, res)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", res.Code, body)
	}

	var out listResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out.Photos) != 3 {
		t.Fatalf("returned %d photos, want 3", len(out.Photos))
	}

	wantOrder := []string{uuidString(first.ID), uuidString(second.ID), uuidString(third.ID)}
	for i, want := range wantOrder {
		if out.Photos[i].ID != want {
			t.Fatalf("photo %d = %s, want %s (list must be ordered by taken_at, not upload order)",
				i, out.Photos[i].ID, want)
		}
	}

	for i, photo := range out.Photos {
		if photo.URL == "" || photo.ThumbURL == "" {
			t.Fatalf("photo %d has an empty url or thumb_url", i)
		}
		if !strings.Contains(photo.URL, "X-Amz-Signature") {
			t.Fatalf("photo %d url is not signed: %s", i, photo.URL)
		}
		if photo.Width != 1920 || photo.Height != 1080 {
			t.Errorf("photo %d dimensions = %dx%d, want 1920x1080", i, photo.Width, photo.Height)
		}
		if photo.URL == photo.ThumbURL {
			t.Errorf("photo %d url and thumb_url are identical", i)
		}
	}
}

func TestOriginalServesTheBytesOnceTheEventIsOver(t *testing.T) {
	h := newHarness(t, afterEvent)
	photo := h.seedPhoto(t, time.Date(2026, 9, 6, 12, 30, 45, 0, time.UTC))
	h.store.Put(context.Background(), photo.StorageKey, strings.NewReader("original-bytes"), "image/jpeg", 14)

	res := h.do(httptest.NewRequest(http.MethodGet, "/photos/"+uuidString(photo.ID)+"/original", nil), true)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", res.Code, readBody(t, res))
	}

	// A 302 to the bucket is what broke downloads in the browser: the redirect
	// leaves this origin, and the bucket sends no CORS headers.
	if location := res.Header().Get("Location"); location != "" {
		t.Fatalf("response redirects to %s; the bytes must come from this origin", location)
	}
	if body := res.Body.String(); body != "original-bytes" {
		t.Fatalf("body = %q, want the stored original", body)
	}

	// This is what makes "download" save to the camera roll rather than open a tab.
	disposition := res.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment", disposition)
	}
	if !strings.Contains(disposition, "italy-20260906-123045.jpg") {
		t.Errorf("download filename is not derived from taken_at: %s", disposition)
	}
	if ct := res.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
}

// A signed bucket URL must never reach the client for an original: following it
// is exactly the cross-origin hop the browser refuses.
func TestOriginalLeaksNoSignedURL(t *testing.T) {
	h := newHarness(t, afterEvent)
	photo := h.seedPhoto(t, time.Date(2026, 9, 6, 12, 30, 45, 0, time.UTC))
	h.store.Put(context.Background(), photo.StorageKey, strings.NewReader("original-bytes"), "image/jpeg", 14)

	res := h.do(httptest.NewRequest(http.MethodGet, "/photos/"+uuidString(photo.ID)+"/original", nil), true)
	for key, values := range res.Header() {
		for _, value := range values {
			if strings.Contains(value, "X-Amz-Signature") {
				t.Fatalf("header %s carries a signed URL: %s", key, value)
			}
		}
	}
}

func TestOriginalIsStillLockedWhileTheEventRuns(t *testing.T) {
	h := newHarness(t, duringEvent)
	photo := h.seedPhoto(t, time.Date(2026, 9, 6, 12, 30, 45, 0, time.UTC))
	h.store.Put(context.Background(), photo.StorageKey, strings.NewReader("original-bytes"), "image/jpeg", 14)

	res := h.do(httptest.NewRequest(http.MethodGet, "/photos/"+uuidString(photo.ID)+"/original", nil), true)
	if res.Code != http.StatusLocked {
		t.Fatalf("status = %d, want 423; body: %s", res.Code, readBody(t, res))
	}
	if body := readBody(t, res); strings.Contains(body, "original-bytes") {
		t.Fatal("the locked response contains the photo bytes")
	}
}

func TestOriginalRejectsUnknownAndMalformedIds(t *testing.T) {
	h := newHarness(t, afterEvent)

	unknown := h.do(httptest.NewRequest(http.MethodGet, "/photos/"+uuid.NewString()+"/original", nil), true)
	if unknown.Code != http.StatusNotFound {
		t.Errorf("unknown id = %d, want 404", unknown.Code)
	}

	malformed := h.do(httptest.NewRequest(http.MethodGet, "/photos/not-a-uuid/original", nil), true)
	if malformed.Code != http.StatusBadRequest {
		t.Errorf("malformed id = %d, want 400", malformed.Code)
	}
}

func TestAlbumIsEmptyListNotNull(t *testing.T) {
	h := newHarness(t, afterEvent)

	res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos", nil), true)
	body := readBody(t, res)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if !strings.Contains(body, `"photos":[]`) {
		t.Fatalf("empty album body = %s, want an empty array rather than null", strings.TrimSpace(body))
	}
}

// TestLockBoundaryIsInclusive pins the exact moment the album opens.
func TestLockBoundaryIsInclusive(t *testing.T) {
	tests := []struct {
		name       string
		now        time.Time
		wantStatus int
	}{
		{"one second before ends_at", testEndsAt.Add(-time.Second), http.StatusLocked},
		{"exactly ends_at", testEndsAt, http.StatusOK},
		{"one second after ends_at", testEndsAt.Add(time.Second), http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.now)
			res := h.do(httptest.NewRequest(http.MethodGet, "/events/current/photos", nil), true)
			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.Code, tt.wantStatus)
			}
		})
	}
}
