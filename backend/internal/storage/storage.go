package storage

import (
	"context"
	"io"
	"time"
)

// Storage is the only way handlers touch object storage. Keeping the AWS SDK
// behind this interface means handler tests need no network and no credentials,
// and it keeps the presigning — the thing the whole lock depends on — in one
// package.
type Storage interface {
	// Put writes body at key. size is the exact byte length; S3 needs it up front.
	Put(ctx context.Context, key string, body io.Reader, contentType string, size int64) error

	// PresignGet mints a temporary GET URL for key.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)

	// Open streams the object at key so the API can serve the bytes itself.
	// The caller closes Body.
	//
	// Downloads cannot go through PresignGet: the browser fetches them with an
	// Authorization header, which makes it a CORS request, and a redirect to
	// the bucket lands on an origin that sends no Access-Control-Allow-Origin.
	Open(ctx context.Context, key string) (*Download, error)
}

// Download is an object's bytes plus what is needed to serve them on.
// ContentLength is -1 when the size is not known up front.
type Download struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
}

// Key helpers keep the layout in one place: originals under photos/, thumbnails
// under thumbs/, keyed by the photo's own UUID.
func PhotoKey(id string) string { return "photos/" + id + ".jpg" }
func ThumbKey(id string) string { return "thumbs/" + id + ".jpg" }
