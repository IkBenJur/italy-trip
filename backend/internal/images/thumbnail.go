// Package images does the one piece of server-side image work this app needs:
// a thumbnail small enough that the album grid is not sixty full-size JPEGs.
//
// Capture happens in-page from a <video> frame, so the uploaded bytes carry no
// EXIF and no HEIC — there is no orientation to correct, no metadata to strip,
// and no exotic decoder to reach for.
package images

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"

	"golang.org/x/image/draw"
)

const (
	// ThumbMaxEdge is the long edge of a generated thumbnail, in pixels. 400
	// stays sharp on a retina grid without pulling megabytes per tile.
	ThumbMaxEdge = 400

	// ThumbQuality trades a little fidelity for size; thumbnails are never the
	// thing anyone looks at closely.
	ThumbQuality = 80

	// MaxPixels caps the decoded bitmap. A modest JPEG can declare enormous
	// dimensions, and decoding it would allocate hundreds of megabytes.
	MaxPixels = 50_000_000
)

// Thumb is the result of processing one uploaded JPEG.
type Thumb struct {
	// Width and Height are the original image's real dimensions, read from the
	// decoded bitmap rather than taken on the client's word.
	Width  int
	Height int

	// JPEG holds the encoded thumbnail.
	JPEG []byte
	// ThumbWidth and ThumbHeight are the thumbnail's dimensions.
	ThumbWidth  int
	ThumbHeight int
}

// Make decodes a JPEG and produces a thumbnail whose long edge is at most
// ThumbMaxEdge, preserving aspect ratio. Images already within that bound are
// re-encoded at their original size rather than upscaled.
//
// It returns an error — never a panic — for anything that is not a decodable
// JPEG.
func Make(data []byte) (Thumb, error) {
	if len(data) == 0 {
		return Thumb{}, fmt.Errorf("image is empty")
	}

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Thumb{}, fmt.Errorf("not a decodable JPEG: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return Thumb{}, fmt.Errorf("JPEG declares empty dimensions %dx%d", cfg.Width, cfg.Height)
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return Thumb{}, fmt.Errorf("JPEG is %dx%d, over the %d pixel limit", cfg.Width, cfg.Height, MaxPixels)
	}

	src, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return Thumb{}, fmt.Errorf("decode JPEG: %w", err)
	}

	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return Thumb{}, fmt.Errorf("decoded image is empty")
	}

	thumbWidth, thumbHeight := FitWithin(width, height, ThumbMaxEdge)

	dst := image.NewRGBA(image.Rect(0, 0, thumbWidth, thumbHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: ThumbQuality}); err != nil {
		return Thumb{}, fmt.Errorf("encode thumbnail: %w", err)
	}

	return Thumb{
		Width:       width,
		Height:      height,
		JPEG:        buf.Bytes(),
		ThumbWidth:  thumbWidth,
		ThumbHeight: thumbHeight,
	}, nil
}

// FitWithin scales width x height so its long edge is at most maxEdge, keeping
// the aspect ratio. Images already inside the bound are returned unchanged —
// upscaling only adds bytes, never detail.
func FitWithin(width, height, maxEdge int) (int, int) {
	longEdge := width
	if height > longEdge {
		longEdge = height
	}
	if longEdge <= maxEdge {
		return width, height
	}

	scale := float64(maxEdge) / float64(longEdge)
	scaled := func(v int) int {
		out := int(math.Round(float64(v) * scale))
		if out < 1 {
			return 1 // never round a thin edge away to nothing
		}
		return out
	}

	return scaled(width), scaled(height)
}
