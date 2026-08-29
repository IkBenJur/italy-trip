package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/IkBenJur/italy-trip/internal/env"
)

// These tests talk to a real S3-compatible endpoint. They are what settle
// path-style addressing and, more importantly, prove the bucket is private —
// the whole product rests on nobody being able to read an object without a
// signature we minted.
//
// Objects are namespaced under test/ so they never collide with real photos.
func newIntegrationStorage(t *testing.T) (*S3Storage, string) {
	t.Helper()

	bucket := os.Getenv("AWS_S3_BUCKET_NAME")
	if bucket == "" || os.Getenv("AWS_ACCESS_KEY_ID") == "" {
		t.Skip("skipping: set AWS_S3_BUCKET_NAME, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_DEFAULT_REGION and AWS_ENDPOINT_URL to run")
	}

	usePathStyle := env.GetEnvBool("AWS_S3_USE_PATH_STYLE", false)
	t.Logf("endpoint=%s bucket=%s path_style=%v", os.Getenv("AWS_ENDPOINT_URL"), bucket, usePathStyle)

	store, err := NewS3FromEnv(context.Background(), bucket, usePathStyle)
	if err != nil {
		t.Fatalf("NewS3FromEnv: %v", err)
	}

	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("rand: %v", err)
	}
	key := "test/roundtrip-" + time.Now().UTC().Format("20060102T150405") + "-" + hex(suffix) + ".bin"

	return store, key
}

func hex(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0x0f])
	}
	return string(out)
}

func TestS3RoundTrip(t *testing.T) {
	store, key := newIntegrationStorage(t)
	ctx := context.Background()

	payload := []byte("italy-trip storage round-trip \x00\x01\x02 binary payload")

	if err := store.Put(ctx, key, bytes.NewReader(payload), "application/octet-stream", int64(len(payload))); err != nil {
		t.Fatalf("Put: %v (if this is a 'not implemented'/'bad request' style error, try AWS_S3_USE_PATH_STYLE=true)", err)
	}

	signed, err := store.PresignGet(ctx, key, time.Hour)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if !strings.Contains(signed, "X-Amz-Signature") {
		t.Fatalf("presigned URL carries no signature: %s", signed)
	}

	body, status := httpGet(t, signed)
	if status != http.StatusOK {
		t.Fatalf("GET presigned URL = %d, want 200; body: %s", status, truncate(body))
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("round-tripped %d bytes, want %d identical bytes", len(body), len(payload))
	}

	t.Run("unsigned GET is refused: the bucket must be private", func(t *testing.T) {
		parsed, err := url.Parse(signed)
		if err != nil {
			t.Fatalf("parse signed URL: %v", err)
		}
		parsed.RawQuery = "" // strip every signature parameter

		body, status := httpGet(t, parsed.String())
		if status == http.StatusOK {
			t.Fatalf("unsigned GET of %s returned 200 — THE BUCKET IS PUBLIC. Every photo is readable without the app.", parsed)
		}
		if status != http.StatusForbidden && status != http.StatusUnauthorized && status != http.StatusNotFound {
			t.Fatalf("unsigned GET = %d, want 403; body: %s", status, truncate(body))
		}
		t.Logf("unsigned GET correctly refused with %d", status)
	})

	t.Run("presigned URL expires", func(t *testing.T) {
		shortLived, err := store.PresignGet(ctx, key, time.Second)
		if err != nil {
			t.Fatalf("PresignGet: %v", err)
		}

		if _, status := httpGet(t, shortLived); status != http.StatusOK {
			t.Fatalf("1s URL = %d before expiry, want 200", status)
		}

		time.Sleep(2 * time.Second)

		body, status := httpGet(t, shortLived)
		if status == http.StatusOK {
			t.Fatalf("expired presigned URL still returned 200 — TTLs are not being enforced")
		}
		if status != http.StatusForbidden {
			t.Logf("expired URL = %d (not 403, but refused); body: %s", status, truncate(body))
		}
	})

	t.Run("download disposition", func(t *testing.T) {
		download, err := store.PresignDownload(ctx, key, "italy.jpg", time.Hour)
		if err != nil {
			t.Fatalf("PresignDownload: %v", err)
		}

		res, err := http.Get(download)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer res.Body.Close()
		io.Copy(io.Discard, res.Body)

		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET download URL = %d, want 200", res.StatusCode)
		}
		if got := res.Header.Get("Content-Disposition"); !strings.Contains(got, "attachment") {
			t.Fatalf("Content-Disposition = %q, want an attachment disposition", got)
		}
	})
}

func httpGet(t *testing.T, rawURL string) ([]byte, int) {
	t.Helper()
	res, err := http.Get(rawURL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return body, res.StatusCode
}

func truncate(b []byte) string {
	if len(b) > 400 {
		return string(b[:400]) + "…"
	}
	return string(b)
}
