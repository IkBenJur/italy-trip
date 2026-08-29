package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"sync"
	"time"
)

// Object is what the fake recorded for one key.
type Object struct {
	Body        []byte
	ContentType string
}

// Fake is an in-memory Storage for handler tests. It is safe for concurrent use
// and can be told to fail, so tests can cover the "storage is down" path without
// touching the network.
type Fake struct {
	mu      sync.Mutex
	objects map[string]Object

	// PutErr, when set, makes every Put fail.
	PutErr error
	// PresignErr, when set, makes every presign fail.
	PresignErr error
}

func NewFake() *Fake {
	return &Fake{objects: map[string]Object{}}
}

var _ Storage = (*Fake)(nil)

func (f *Fake) Put(ctx context.Context, key string, body io.Reader, contentType string, size int64) error {
	if f.PutErr != nil {
		return f.PutErr
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = Object{Body: data, ContentType: contentType}
	return nil
}

func (f *Fake) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if f.PresignErr != nil {
		return "", f.PresignErr
	}
	// Shaped like a real presigned URL so tests that grep a response body for
	// "X-Amz-Signature" catch a leak from the fake too.
	return fmt.Sprintf(
		"https://fake-bucket.example.com/%s?X-Amz-Expires=%d&X-Amz-Signature=fakesignature",
		key, int(ttl.Seconds()),
	), nil
}

func (f *Fake) PresignDownload(ctx context.Context, key, filename string, ttl time.Duration) (string, error) {
	signed, err := f.PresignGet(ctx, key, ttl)
	if err != nil {
		return "", err
	}
	return signed + "&response-content-disposition=" + url.QueryEscape(contentDisposition(filename)), nil
}

// Get returns a stored object, for assertions.
func (f *Fake) Get(key string) (Object, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	obj, ok := f.objects[key]
	return obj, ok
}

// Keys returns every stored key, sorted.
func (f *Fake) Keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.objects))
	for key := range f.objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Len is the number of stored objects.
func (f *Fake) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objects)
}
