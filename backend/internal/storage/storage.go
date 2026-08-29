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

	// PresignDownload is PresignGet with response-content-disposition set to
	// attachment, which is what makes a browser save the file rather than open
	// it in a tab.
	PresignDownload(ctx context.Context, key, filename string, ttl time.Duration) (string, error)
}

// Key helpers keep the layout in one place: originals under photos/, thumbnails
// under thumbs/, keyed by the photo's own UUID.
func PhotoKey(id string) string { return "photos/" + id + ".jpg" }
func ThumbKey(id string) string { return "thumbs/" + id + ".jpg" }
