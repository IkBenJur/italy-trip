package photos

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func decodeUpload(t *testing.T, body string) uploadResponse {
	t.Helper()
	var out uploadResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode upload response %q: %v", body, err)
	}
	return out
}

func TestUploadHappyPath(t *testing.T) {
	h := newHarness(t, duringEvent)

	clientID := uuid.NewString()
	req := uploadRequest(t, "capture.jpg", jpegBytes(t, 1920, 1080), clientID, "2026-09-06T14:00:00+02:00")
	res := h.do(req, true)

	body := readBody(t, res)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", res.Code, body)
	}

	created := decodeUpload(t, body)
	if created.ID == "" {
		t.Fatal("response carries no photo id")
	}
	if created.Duplicate {
		t.Error("a first upload should not be marked duplicate")
	}

	if got := h.queries.photoCount(); got != 1 {
		t.Fatalf("photo rows = %d, want 1", got)
	}

	// One original and one thumbnail, both under the expected prefixes.
	keys := h.store.Keys()
	if len(keys) != 2 {
		t.Fatalf("stored objects = %v, want exactly 2", keys)
	}
	wantOriginal := "photos/" + created.ID + ".jpg"
	wantThumb := "thumbs/" + created.ID + ".jpg"
	if keys[0] != wantOriginal || keys[1] != wantThumb {
		t.Fatalf("stored keys = %v, want [%s %s]", keys, wantOriginal, wantThumb)
	}

	original, _ := h.store.Get(wantOriginal)
	thumb, _ := h.store.Get(wantThumb)
	if original.ContentType != "image/jpeg" || thumb.ContentType != "image/jpeg" {
		t.Errorf("content types = %s/%s, want image/jpeg for both", original.ContentType, thumb.ContentType)
	}
	if len(thumb.Body) >= len(original.Body) {
		t.Errorf("thumbnail (%d bytes) is not smaller than the original (%d bytes)", len(thumb.Body), len(original.Body))
	}

	// The row must record the real decoded dimensions, not the client's word.
	stored := h.queries.photos[0]
	if stored.Width != 1920 || stored.Height != 1080 {
		t.Errorf("stored dimensions = %dx%d, want 1920x1080", stored.Width, stored.Height)
	}
	if stored.SizeBytes != int64(len(original.Body)) {
		t.Errorf("size_bytes = %d, want %d", stored.SizeBytes, len(original.Body))
	}
	want := time.Date(2026, 9, 6, 14, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	if !stored.TakenAt.Time.Equal(want) {
		t.Errorf("taken_at = %s, want the same instant as %s", stored.TakenAt.Time, want)
	}

	// Nothing in the upload response may be a route back to the bytes.
	assertNoLeak(t, body)
}

func TestUploadIsIdempotentOnClientID(t *testing.T) {
	h := newHarness(t, duringEvent)
	clientID := uuid.NewString()
	image := jpegBytes(t, 1280, 720)

	first := h.do(uploadRequest(t, "a.jpg", image, clientID, "2026-09-06T14:00:00+02:00"), true)
	if first.Code != http.StatusCreated {
		t.Fatalf("first upload = %d, want 201; body: %s", first.Code, readBody(t, first))
	}
	firstID := decodeUpload(t, readBody(t, first)).ID

	// The retry sends different bytes under the same client_id — the server must
	// still treat it as the same capture and not overwrite anything.
	second := h.do(uploadRequest(t, "a.jpg", jpegBytes(t, 640, 480), clientID, "2026-09-06T14:00:00+02:00"), true)
	secondBody := readBody(t, second)
	if second.Code != http.StatusOK {
		t.Fatalf("retry = %d, want 200; body: %s", second.Code, secondBody)
	}

	retry := decodeUpload(t, secondBody)
	if retry.ID != firstID {
		t.Errorf("retry returned id %s, want the original %s", retry.ID, firstID)
	}
	if !retry.Duplicate {
		t.Error("retry should be marked duplicate")
	}

	if got := h.queries.photoCount(); got != 1 {
		t.Fatalf("photo rows after retry = %d, want 1", got)
	}
	if keys := h.store.Keys(); len(keys) != 2 {
		t.Fatalf("stored objects after retry = %v, want exactly 2", keys)
	}
	assertNoLeak(t, secondBody)
}

// TestUploadRacingRetries covers two retries of the same capture arriving at
// once: the lookup misses for both, and the unique constraint decides.
func TestUploadRacingRetries(t *testing.T) {
	h := newHarness(t, duringEvent)
	clientID := uuid.NewString()

	// Slip a competing row in after the lookup but before the insert.
	var raced bool
	h.queries.beforeCreate = func() {
		if raced {
			return
		}
		raced = true
		h.queries.beforeCreate = nil
		other := h.do(uploadRequest(t, "b.jpg", jpegBytes(t, 800, 600), clientID, "2026-09-06T14:00:00+02:00"), true)
		if other.Code != http.StatusCreated {
			t.Errorf("competing upload = %d, want 201", other.Code)
		}
	}

	res := h.do(uploadRequest(t, "a.jpg", jpegBytes(t, 800, 600), clientID, "2026-09-06T14:00:00+02:00"), true)
	body := readBody(t, res)
	if res.Code != http.StatusOK {
		t.Fatalf("loser of the race = %d, want 200; body: %s", res.Code, body)
	}
	if !decodeUpload(t, body).Duplicate {
		t.Error("loser of the race should be marked duplicate")
	}
	if got := h.queries.photoCount(); got != 1 {
		t.Fatalf("photo rows after race = %d, want 1", got)
	}
}

func TestUploadRefusedOnceEventIsOver(t *testing.T) {
	h := newHarness(t, afterEvent)

	res := h.do(uploadRequest(t, "late.jpg", jpegBytes(t, 800, 600), uuid.NewString(), "2026-09-06T14:00:00+02:00"), true)
	if res.Code != http.StatusLocked {
		t.Fatalf("status = %d, want 423; body: %s", res.Code, readBody(t, res))
	}
	if got := h.queries.photoCount(); got != 0 {
		t.Errorf("photo rows = %d, want 0", got)
	}
	if got := h.store.Len(); got != 0 {
		t.Errorf("stored objects = %d, want 0", got)
	}
}

func TestUploadRejectsOversizedBody(t *testing.T) {
	h := newHarness(t, duringEvent)
	h.handler.MaxUploadBytes = 1 << 20 // 1 MB, so the fixture stays cheap to build

	oversized := bytes.Repeat([]byte{0xab}, 20<<20) // 20 MB
	res := h.do(uploadRequest(t, "huge.jpg", oversized, uuid.NewString(), "2026-09-06T14:00:00+02:00"), true)

	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body: %s", res.Code, readBody(t, res))
	}
	if got := h.store.Len(); got != 0 {
		t.Errorf("stored objects = %d, want 0: nothing may be written for a rejected body", got)
	}
	if got := h.queries.photoCount(); got != 0 {
		t.Errorf("photo rows = %d, want 0", got)
	}
}

func TestUploadRejectsBadInput(t *testing.T) {
	validImage := jpegBytes(t, 800, 600)

	tests := []struct {
		name     string
		content  []byte
		filename string
		clientID string
		takenAt  string
	}{
		{
			name:     "a PNG",
			content:  nil, // filled in below
			filename: "sneaky.jpg",
			clientID: uuid.NewString(),
			takenAt:  "2026-09-06T14:00:00+02:00",
		},
		{
			name:     "text named .jpg",
			content:  []byte("this is a text file wearing a jpg extension, nothing more"),
			filename: "notreally.jpg",
			clientID: uuid.NewString(),
			takenAt:  "2026-09-06T14:00:00+02:00",
		},
		{
			name:     "truncated JPEG",
			content:  validImage[:40],
			filename: "cut.jpg",
			clientID: uuid.NewString(),
			takenAt:  "2026-09-06T14:00:00+02:00",
		},
		{
			name:     "missing client_id",
			content:  validImage,
			filename: "a.jpg",
			clientID: "",
			takenAt:  "2026-09-06T14:00:00+02:00",
		},
		{
			name:     "client_id is not a UUID",
			content:  validImage,
			filename: "a.jpg",
			clientID: "not-a-uuid",
			takenAt:  "2026-09-06T14:00:00+02:00",
		},
		{
			name:     "missing taken_at",
			content:  validImage,
			filename: "a.jpg",
			clientID: uuid.NewString(),
			takenAt:  "",
		},
		{
			name:     "taken_at without an offset",
			content:  validImage,
			filename: "a.jpg",
			clientID: uuid.NewString(),
			takenAt:  "2026-09-06T14:00:00",
		},
		{
			name:     "no file part at all",
			content:  nil,
			filename: "",
			clientID: uuid.NewString(),
			takenAt:  "2026-09-06T14:00:00+02:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, duringEvent)

			content := tt.content
			if tt.name == "a PNG" {
				content = pngBytes(t)
			}

			res := h.do(uploadRequest(t, tt.filename, content, tt.clientID, tt.takenAt), true)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", res.Code, readBody(t, res))
			}
			if got := h.store.Len(); got != 0 {
				t.Errorf("stored objects = %d, want 0", got)
			}
			if got := h.queries.photoCount(); got != 0 {
				t.Errorf("photo rows = %d, want 0", got)
			}
		})
	}
}

func TestUploadRequiresAuth(t *testing.T) {
	h := newHarness(t, duringEvent)

	req := uploadRequest(t, "a.jpg", jpegBytes(t, 800, 600), uuid.NewString(), "2026-09-06T14:00:00+02:00")
	res := h.do(req, false)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", res.Code, readBody(t, res))
	}
	if h.store.Len() != 0 || h.queries.photoCount() != 0 {
		t.Error("an unauthenticated request must not write anything")
	}
}
