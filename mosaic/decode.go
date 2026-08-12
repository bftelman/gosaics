package mosaic

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"

	// Registers the PNG decoder with image.Decode. The JPEG decoder is
	// registered by the image/jpeg import above, which EncodeJPEG also uses
	// directly. These are the only two formats gosaics supports, so no other
	// image format package is imported anywhere in this package.
	_ "image/png"
)

// JPEGQuality is the quality setting used when encoding output mosaics.
const JPEGQuality = 90

// ErrUnsupportedFormat means the data was not JPEG or PNG.
var ErrUnsupportedFormat = errors.New("unsupported image format: only JPEG and PNG are supported")

// Decode reads a JPEG or PNG image from r. Data in any other format is
// reported as ErrUnsupportedFormat.
func Decode(r io.Reader) (image.Image, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return nil, ErrUnsupportedFormat
		}
		return nil, fmt.Errorf("decoding image: %w", err)
	}
	return img, nil
}

// EncodeJPEG writes img to w as a JPEG at JPEGQuality.
func EncodeJPEG(w io.Writer, img image.Image) error {
	if err := jpeg.Encode(w, img, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		return fmt.Errorf("encoding JPEG: %w", err)
	}
	return nil
}
