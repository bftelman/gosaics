package mosaic

import (
	"bytes"
	"errors"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestDecode_JPEG(t *testing.T) {
	var buf bytes.Buffer
	src := solidImage(16, 16, color.RGBA{200, 100, 50, 255})
	if err := jpeg.Encode(&buf, src, nil); err != nil {
		t.Fatalf("encoding fixture failed: %v", err)
	}

	got, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if got.Bounds().Dx() != 16 || got.Bounds().Dy() != 16 {
		t.Errorf("decoded bounds %v, want 16x16", got.Bounds())
	}
}

func TestDecode_PNG(t *testing.T) {
	var buf bytes.Buffer
	src := solidImage(8, 12, color.RGBA{0, 255, 0, 255})
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("encoding fixture failed: %v", err)
	}

	got, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if got.Bounds().Dx() != 8 || got.Bounds().Dy() != 12 {
		t.Errorf("decoded bounds %v, want 8x12", got.Bounds())
	}
}

func TestDecode_UnsupportedFormat(t *testing.T) {
	// GIF is a real image format we deliberately do not support. The magic
	// header is written by hand rather than via image/gif on purpose: importing
	// image/gif anywhere in this package would register its decoder with
	// image.Decode globally and make this case succeed.
	gifHeader := []byte("GIF89a\x04\x00\x04\x00\x80\x00\x00")

	if _, err := Decode(bytes.NewReader(gifHeader)); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("got error %v, want ErrUnsupportedFormat", err)
	}
}

func TestDecode_GarbageInput(t *testing.T) {
	if _, err := Decode(strings.NewReader("this is definitely not an image")); err == nil {
		t.Error("Decode accepted garbage input, want an error")
	}
}

func TestDecode_EmptyInput(t *testing.T) {
	if _, err := Decode(bytes.NewReader(nil)); err == nil {
		t.Error("Decode accepted empty input, want an error")
	}
}

func TestEncodeJPEG_RoundTrips(t *testing.T) {
	src := solidImage(20, 20, color.RGBA{123, 45, 67, 255})

	var buf bytes.Buffer
	if err := EncodeJPEG(&buf, src); err != nil {
		t.Fatalf("EncodeJPEG returned error: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("EncodeJPEG wrote no bytes")
	}

	got, err := Decode(&buf)
	if err != nil {
		t.Fatalf("re-decoding encoded output failed: %v", err)
	}
	if got.Bounds().Dx() != 20 || got.Bounds().Dy() != 20 {
		t.Errorf("round-tripped bounds %v, want 20x20", got.Bounds())
	}
	// JPEG is lossy, so allow a generous tolerance on a solid color.
	avg := AverageRGB(got)
	if absDiff(avg.R, 123) > 10 || absDiff(avg.G, 45) > 10 || absDiff(avg.B, 67) > 10 {
		t.Errorf("round-tripped average %+v, want near {123 45 67}", avg)
	}
}

func TestDecode_TruncatedValidFormatIsNotUnsupportedFormat(t *testing.T) {
	// A file with real JPEG magic bytes that is then cut short must be
	// reported as a decode failure, NOT as an unsupported format. Callers
	// branch on this distinction to tell a corrupt upload from a wrong one.
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solidImage(32, 32, color.RGBA{200, 100, 50, 255}), nil); err != nil {
		t.Fatalf("encoding fixture failed: %v", err)
	}

	full := buf.Bytes()
	truncated := full[:len(full)/2]

	_, err := Decode(bytes.NewReader(truncated))
	if err == nil {
		t.Fatal("Decode accepted a truncated JPEG, want an error")
	}
	if errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("truncated JPEG reported as ErrUnsupportedFormat; want a wrapped decode error, got %v", err)
	}
}
