package images

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// testJPEG builds a JPEG of the given size with some structure in it, so
// resampling has something real to work on.
func testJPEG(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / max(width-1, 1)),
				G: uint8((y * 255) / max(height-1, 1)),
				B: uint8((x ^ y) & 0xff),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return buf.Bytes()
}

func TestMakeDimensions(t *testing.T) {
	tests := []struct {
		name                   string
		width, height          int
		wantThumbW, wantThumbH int
	}{
		{name: "landscape 1080p", width: 1920, height: 1080, wantThumbW: 400, wantThumbH: 225},
		{name: "portrait 1080p", width: 1080, height: 1920, wantThumbW: 225, wantThumbH: 400},
		{name: "square", width: 1000, height: 1000, wantThumbW: 400, wantThumbH: 400},
		{name: "exactly at the bound is untouched", width: 400, height: 300, wantThumbW: 400, wantThumbH: 300},
		{name: "smaller than the bound is not upscaled", width: 320, height: 240, wantThumbW: 320, wantThumbH: 240},
		{name: "tiny is not upscaled", width: 10, height: 10, wantThumbW: 10, wantThumbH: 10},
		{name: "extreme panorama keeps a visible short edge", width: 4000, height: 40, wantThumbW: 400, wantThumbH: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thumb, err := Make(testJPEG(t, tt.width, tt.height))
			if err != nil {
				t.Fatalf("Make: %v", err)
			}

			if thumb.Width != tt.width || thumb.Height != tt.height {
				t.Errorf("original dimensions = %dx%d, want %dx%d",
					thumb.Width, thumb.Height, tt.width, tt.height)
			}
			if thumb.ThumbWidth != tt.wantThumbW || thumb.ThumbHeight != tt.wantThumbH {
				t.Errorf("thumbnail dimensions = %dx%d, want %dx%d",
					thumb.ThumbWidth, thumb.ThumbHeight, tt.wantThumbW, tt.wantThumbH)
			}

			// The bytes must be a real, decodable JPEG of exactly that size.
			decoded, err := jpeg.Decode(bytes.NewReader(thumb.JPEG))
			if err != nil {
				t.Fatalf("thumbnail is not decodable JPEG: %v", err)
			}
			if got := decoded.Bounds(); got.Dx() != tt.wantThumbW || got.Dy() != tt.wantThumbH {
				t.Errorf("decoded thumbnail = %dx%d, want %dx%d",
					got.Dx(), got.Dy(), tt.wantThumbW, tt.wantThumbH)
			}
		})
	}
}

func TestMakeShrinksBytes(t *testing.T) {
	original := testJPEG(t, 1920, 1080)
	thumb, err := Make(original)
	if err != nil {
		t.Fatalf("Make: %v", err)
	}
	if len(thumb.JPEG) >= len(original) {
		t.Fatalf("thumbnail is %d bytes, not smaller than the %d byte original",
			len(thumb.JPEG), len(original))
	}
}

func TestMakeRejectsNonJPEG(t *testing.T) {
	var pngBuf bytes.Buffer
	pngImage := image.NewRGBA(image.Rect(0, 0, 64, 64))
	if err := png.Encode(&pngBuf, pngImage); err != nil {
		t.Fatalf("encode png fixture: %v", err)
	}

	valid := testJPEG(t, 200, 200)

	tests := []struct {
		name string
		data []byte
	}{
		{name: "PNG", data: pngBuf.Bytes()},
		{name: "plain text", data: []byte("this is definitely not a jpeg, it is a text file")},
		{name: "empty", data: nil},
		{name: "single byte", data: []byte{0xff}},
		{name: "JPEG magic bytes only", data: []byte{0xff, 0xd8, 0xff}},
		{name: "truncated JPEG", data: valid[:len(valid)/3]},
		{name: "JPEG with a corrupted tail", data: append(append([]byte{}, valid[:len(valid)-20]...), bytes.Repeat([]byte{0x00}, 20)...)},
		{name: "random bytes", data: bytes.Repeat([]byte{0x13, 0x37, 0xab}, 500)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The point is that nothing here panics; an error is the contract.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Make panicked on %s input: %v", tt.name, r)
				}
			}()

			if _, err := Make(tt.data); err == nil {
				t.Fatalf("Make(%s) returned no error, want one", tt.name)
			}
		})
	}
}

func TestFitWithin(t *testing.T) {
	tests := []struct {
		w, h, maxEdge int
		wantW, wantH  int
	}{
		{1920, 1080, 400, 400, 225},
		{1080, 1920, 400, 225, 400},
		{1000, 1000, 400, 400, 400},
		{399, 399, 400, 399, 399},
		{800, 400, 400, 400, 200},
		{4000, 40, 400, 400, 4},
		{40, 4000, 400, 4, 400},
		{100000, 1, 400, 400, 1},
	}

	for _, tt := range tests {
		gotW, gotH := FitWithin(tt.w, tt.h, tt.maxEdge)
		if gotW != tt.wantW || gotH != tt.wantH {
			t.Errorf("FitWithin(%d, %d, %d) = %dx%d, want %dx%d",
				tt.w, tt.h, tt.maxEdge, gotW, gotH, tt.wantW, tt.wantH)
		}
	}
}
