package photos

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/IkBenJur/italy-trip/internal/auth"
	"github.com/IkBenJur/italy-trip/internal/middleware"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func init() { gin.SetMode(gin.TestMode) }

// The trip ends at 2026-09-14T23:59:59+02:00; tests move `now` either side of it.
var (
	testEndsAt   = time.Date(2026, 9, 14, 23, 59, 59, 0, time.FixedZone("CEST", 2*60*60))
	testStartsAt = time.Date(2026, 9, 5, 0, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	duringEvent  = testEndsAt.Add(-48 * time.Hour)
	afterEvent   = testEndsAt.Add(time.Hour)
)

type harness struct {
	router  *gin.Engine
	queries *fakeQuerier
	store   *storage.Fake
	handler *Handler
	token   string
	eventID pgtype.UUID
	userID  pgtype.UUID
}

// newHarness builds the real router — CORS off, but the genuine RequireAuth
// middleware — so auth and lock behaviour are exercised together.
func newHarness(t *testing.T, now time.Time) *harness {
	t.Helper()

	eventID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	userID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	queries := newFakeQuerier(repo.Event{
		ID:       eventID,
		Name:     "Italy Trip",
		StartsAt: pgtype.Timestamptz{Time: testStartsAt, Valid: true},
		EndsAt:   pgtype.Timestamptz{Time: testEndsAt, Valid: true},
	})
	queries.addUser(repo.User{ID: userID, Email: "us@example.com", PasswordHash: "x"})

	store := storage.NewFake()
	handler := NewHandler(queries, store, DefaultMaxUploadBytes)
	handler.Now = func() time.Time { return now }

	issuer := auth.NewTokenIssuer("test-secret", time.Hour)
	token, err := issuer.Issue(uuidString(userID))
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	router := gin.New()
	authorized := router.Group("/", middleware.RequireAuth(issuer, queries))
	authorized.POST("events/current/photos", handler.Upload)
	authorized.GET("events/current/photos", handler.List)
	authorized.GET("photos/:id/original", handler.Original)

	return &harness{
		router:  router,
		queries: queries,
		store:   store,
		handler: handler,
		token:   token,
		eventID: eventID,
		userID:  userID,
	}
}

func uuidString(id pgtype.UUID) string {
	parsed, _ := uuid.FromBytes(id.Bytes[:])
	return parsed.String()
}

// do sends req through the router with the harness's bearer token unless
// withAuth is false.
func (h *harness) do(req *http.Request, withAuth bool) *httptest.ResponseRecorder {
	if withAuth {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	res := httptest.NewRecorder()
	h.router.ServeHTTP(res, req)
	return res
}

// uploadRequest builds a multipart POST. A nil body part is omitted entirely.
func uploadRequest(t *testing.T, filename string, content []byte, clientID, takenAt string) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if content != nil {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	if clientID != "" {
		writer.WriteField("client_id", clientID)
	}
	if takenAt != "" {
		writer.WriteField("taken_at", takenAt)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/events/current/photos", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func jpegBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 64))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// assertNoLeak is the check the whole product rests on: a response must never
// carry a storage key, a bucket name, or anything that looks like a signed URL.
// It is applied to locked responses and to responses that are allowed but still
// have no business emitting a route back to the bytes.
func assertNoLeak(t *testing.T, body string) {
	t.Helper()

	forbidden := []string{
		"X-Amz-Signature",
		"X-Amz-Credential",
		"x-amz-signature",
		"fake-bucket.example.com",
		"photos/",
		"thumbs/",
		"storage_key",
		"thumb_key",
		"https://",
	}

	lower := strings.ToLower(body)
	for _, needle := range forbidden {
		if strings.Contains(lower, strings.ToLower(needle)) {
			t.Fatalf("response leaked %q; full body: %s", needle, body)
		}
	}
}

func readBody(t *testing.T, res *httptest.ResponseRecorder) string {
	t.Helper()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}
