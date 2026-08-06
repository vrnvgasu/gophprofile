// Package images - утилиты для валидации и обработки изображений.
package images

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/disintegration/imaging"

	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

var ErrUnsupportedFormat = errors.New("images: unsupported format")

// thumbnailQuality — качество JPEG-сжатия для миниатюр.
const thumbnailQuality = 85

var supportedMimeTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// Info - распознанное изображение.
type Info struct {
	MimeType  string
	Extension string
	Width     int
	Height    int
}

// Detect - формат и размеры изображения по его содержимому.
func Detect(data []byte) (Info, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Info{}, fmt.Errorf("%w: %w", ErrUnsupportedFormat, err)
	}

	mimeType := "image/" + format
	ext, ok := supportedMimeTypes[mimeType]
	if !ok {
		return Info{}, fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}

	return Info{
		MimeType:  mimeType,
		Extension: ext,
		Width:     cfg.Width,
		Height:    cfg.Height,
	}, nil
}

// Thumbnail масштабирует изображение под размер width x height, обрезая лишнее по центру.
func Thumbnail(data []byte, width, height int) ([]byte, error) {
	src, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("images.Thumbnail Decode: %w", err)
	}

	dst := imaging.Fill(src, width, height, imaging.Center, imaging.Lanczos)

	var buf bytes.Buffer
	if err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: thumbnailQuality}); err != nil {
		return nil, fmt.Errorf("images.Thumbnail Encode: %w", err)
	}

	return buf.Bytes(), nil
}
