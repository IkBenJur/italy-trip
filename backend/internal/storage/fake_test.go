package storage

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestFakeRoundTrip(t *testing.T) {
	fake := NewFake()
	ctx := context.Background()

	if err := fake.Put(ctx, PhotoKey("abc"), strings.NewReader("bytes"), "image/jpeg", 5); err != nil {
		t.Fatalf("Put: %v", err)
	}

	obj, ok := fake.Get("photos/abc.jpg")
	if !ok {
		t.Fatalf("Get: key not found, have %v", fake.Keys())
	}
	if string(obj.Body) != "bytes" || obj.ContentType != "image/jpeg" {
		t.Fatalf("Get = %q/%s, want bytes/image/jpeg", obj.Body, obj.ContentType)
	}

	signed, err := fake.PresignGet(ctx, "photos/abc.jpg", time.Hour)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if !strings.Contains(signed, "X-Amz-Signature") {
		t.Fatalf("fake presigned URL %q should look like a signed URL", signed)
	}

	opened, err := fake.Open(ctx, "photos/abc.jpg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer opened.Body.Close()
	got, err := io.ReadAll(opened.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "bytes" || opened.ContentType != "image/jpeg" || opened.ContentLength != 5 {
		t.Fatalf("Open = %q/%s/%d, want bytes/image/jpeg/5", got, opened.ContentType, opened.ContentLength)
	}

	if _, err := fake.Open(ctx, "photos/missing.jpg"); err == nil {
		t.Fatal("Open of a missing key should fail")
	}
}

func TestContentDisposition(t *testing.T) {
	tests := []struct{ in, want string }{
		{"photo.jpg", `attachment; filename="photo.jpg"`},
		{"../../etc/passwd", `attachment; filename="passwd"`},
		{`a"b.jpg`, `attachment; filename="a-b.jpg"`},
		{"with\nnewline.jpg", `attachment; filename="with-newline.jpg"`},
		{"", `attachment; filename="photo.jpg"`},
		{"   ", `attachment; filename="photo.jpg"`},
	}

	for _, tt := range tests {
		if got := ContentDisposition(tt.in); got != tt.want {
			t.Errorf("ContentDisposition(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
