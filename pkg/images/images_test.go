package images

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		for y := range height {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 200, A: 255})
		}
	}

	var buf bytes.Buffer
	switch format {
	case "png":
		require.NoError(t, png.Encode(&buf, img))
	case "jpeg":
		require.NoError(t, jpeg.Encode(&buf, img, nil))
	default:
		t.Fatalf("unknown format %q", format)
	}

	return buf.Bytes()
}

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		data             []byte
		expectedMimeType string
		expectedExt      string
		wantErr          bool
	}{
		{
			name:             "PNG",
			data:             testImage(t, "png", 40, 20),
			expectedMimeType: "image/png",
			expectedExt:      ".png",
		},
		{
			name:             "JPEG",
			data:             testImage(t, "jpeg", 40, 20),
			expectedMimeType: "image/jpeg",
			expectedExt:      ".jpg",
		},
		{
			name:    "Plain text",
			data:    []byte("definitely not an image"),
			wantErr: true,
		},
		{
			name:    "Empty input",
			data:    nil,
			wantErr: true,
		},
		{
			name: "PDF with a fake extension",
			// Файл с сигнатурой PDF не должен пройти проверку, как бы он ни назывался.
			data:    []byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			info, err := Detect(tt.data)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrUnsupportedFormat)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedMimeType, info.MimeType)
			assert.Equal(t, tt.expectedExt, info.Extension)
			assert.Equal(t, 40, info.Width)
			assert.Equal(t, 20, info.Height)
		})
	}
}

func TestThumbnail(t *testing.T) {
	t.Parallel()

	t.Run("Keeps the requested size", func(t *testing.T) {
		t.Parallel()

		// Исходник неквадратный: проверяем, что лишнее обрезается, а не сжимается.
		data, err := Thumbnail(testImage(t, "png", 400, 200), 100, 100)
		require.NoError(t, err)

		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		require.NoError(t, err)
		assert.Equal(t, "jpeg", format)
		assert.Equal(t, 100, cfg.Width)
		assert.Equal(t, 100, cfg.Height)
	})

	t.Run("Upscales a small image", func(t *testing.T) {
		t.Parallel()

		data, err := Thumbnail(testImage(t, "png", 40, 40), 300, 300)
		require.NoError(t, err)

		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		require.NoError(t, err)
		assert.Equal(t, 300, cfg.Width)
	})

	t.Run("Broken input", func(t *testing.T) {
		t.Parallel()

		_, err := Thumbnail([]byte("not an image"), 100, 100)
		require.Error(t, err)
	})
}
