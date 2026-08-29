package storage

import (
	"context"
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

	download, err := fake.PresignDownload(ctx, "photos/abc.jpg", "abc.jpg", time.Hour)
	if err != nil {
		t.Fatalf("PresignDownload: %v", err)
	}
	if !strings.Contains(download, "attachment") {
		t.Fatalf("download URL %q should carry an attachment disposition", download)
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
		if got := contentDisposition(tt.in); got != tt.want {
			t.Errorf("contentDisposition(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
