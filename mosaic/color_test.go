package mosaic

import (
	"image/color"
	"math"
	"testing"
)

const epsilon = 0.01

func TestAverageRGB_SolidColors(t *testing.T) {
	tests := []struct {
		name string
		c    color.RGBA
		want RGB
	}{
		{"black", color.RGBA{0, 0, 0, 255}, RGB{0, 0, 0}},
		{"white", color.RGBA{255, 255, 255, 255}, RGB{255, 255, 255}},
		{"pure red", color.RGBA{255, 0, 0, 255}, RGB{255, 0, 0}},
		{"pure green", color.RGBA{0, 255, 0, 255}, RGB{0, 255, 0}},
		{"pure blue", color.RGBA{0, 0, 255, 255}, RGB{0, 0, 255}},
		{"mid grey", color.RGBA{128, 128, 128, 255}, RGB{128, 128, 128}},
		{"mixed", color.RGBA{10, 20, 30, 255}, RGB{10, 20, 30}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AverageRGB(solidImage(4, 4, tt.c))
			assertRGBNear(t, got, tt.want)
		})
	}
}

func TestAverageRGB_AveragesAcrossPixels(t *testing.T) {
	// Left half black, right half white -> average is mid grey on every channel.
	img := halfAndHalfImage(10, 4, color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 255})
	assertRGBNear(t, AverageRGB(img), RGB{127.5, 127.5, 127.5})
}

func TestAverageRGB_SinglePixel(t *testing.T) {
	assertRGBNear(t, AverageRGB(solidImage(1, 1, color.RGBA{7, 8, 9, 255})), RGB{7, 8, 9})
}

func TestAverageRGB_EmptyImageReturnsZero(t *testing.T) {
	// A zero-area image must not divide by zero.
	assertRGBNear(t, AverageRGB(solidImage(0, 0, color.RGBA{})), RGB{0, 0, 0})
}

func TestSquaredDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b RGB
		want float64
	}{
		{"identical is zero", RGB{10, 20, 30}, RGB{10, 20, 30}, 0},
		{"black to white", RGB{0, 0, 0}, RGB{255, 255, 255}, 3 * 255 * 255},
		{"single channel", RGB{0, 0, 0}, RGB{3, 0, 0}, 9},
		{"all channels", RGB{1, 2, 3}, RGB{4, 6, 8}, 9 + 16 + 25},
		{"symmetric reverse", RGB{4, 6, 8}, RGB{1, 2, 3}, 9 + 16 + 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SquaredDistance(tt.a, tt.b)
			if math.Abs(got-tt.want) > epsilon {
				t.Errorf("SquaredDistance(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func assertRGBNear(t *testing.T, got, want RGB) {
	t.Helper()
	if math.Abs(got.R-want.R) > epsilon ||
		math.Abs(got.G-want.G) > epsilon ||
		math.Abs(got.B-want.B) > epsilon {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
