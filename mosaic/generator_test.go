package mosaic

import (
	"errors"
	"image"
	"image/color"
	"testing"
)

func TestGenerate_OutputDimensions(t *testing.T) {
	tests := []struct {
		name         string
		inW, inH     int
		gridSize     int
		wantW, wantH int
	}{
		// Output is cellSize * gridSize, so non-divisible inputs shrink slightly.
		{"evenly divisible", 100, 100, 10, 100, 100},
		{"rectangular", 200, 100, 10, 200, 100},
		{"non-divisible trims edges", 105, 105, 10, 100, 100},
		{"minimum grid size", 100, 100, 2, 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := solidImage(tt.inW, tt.inH, color.RGBA{128, 128, 128, 255})
			tiles := []image.Image{solidImage(8, 8, color.RGBA{128, 128, 128, 255})}

			out, err := Generate(input, tiles, tt.gridSize)
			if err != nil {
				t.Fatalf("Generate returned error: %v", err)
			}

			b := out.Bounds()
			if b.Dx() != tt.wantW || b.Dy() != tt.wantH {
				t.Errorf("output is %dx%d, want %dx%d", b.Dx(), b.Dy(), tt.wantW, tt.wantH)
			}
		})
	}
}

func TestGenerate_PicksNearestColorTile(t *testing.T) {
	// Input: left half pure red, right half pure blue.
	// Tiles: one pure red, one pure blue, plus a distractor green.
	// Every left-column cell must resolve to red, every right-column cell to blue.
	red := color.RGBA{255, 0, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}
	green := color.RGBA{0, 255, 0, 255}

	input := halfAndHalfImage(40, 40, red, blue)
	tiles := []image.Image{
		solidImage(10, 10, red),
		solidImage(10, 10, blue),
		solidImage(10, 10, green),
	}

	out, err := Generate(input, tiles, 2)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Sample the center of each quadrant of the 40x40 output.
	cases := []struct {
		name string
		x, y int
		want RGB
	}{
		{"top-left is red", 10, 10, RGB{255, 0, 0}},
		{"top-right is blue", 30, 10, RGB{0, 0, 255}},
		{"bottom-left is red", 10, 30, RGB{255, 0, 0}},
		{"bottom-right is blue", 30, 30, RGB{0, 0, 255}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, g, b, _ := out.At(c.x, c.y).RGBA()
			got := RGB{float64(r >> 8), float64(g >> 8), float64(b >> 8)}
			// Resampling can shift values slightly; allow a loose tolerance.
			if absDiff(got.R, c.want.R) > 20 ||
				absDiff(got.G, c.want.G) > 20 ||
				absDiff(got.B, c.want.B) > 20 {
				t.Errorf("pixel (%d,%d) = %+v, want near %+v", c.x, c.y, got, c.want)
			}
		})
	}
}

func TestGenerate_ReusesTilesAcrossCells(t *testing.T) {
	// A single tile must be reused for all 100 cells without error.
	input := solidImage(100, 100, color.RGBA{50, 60, 70, 255})
	tiles := []image.Image{solidImage(10, 10, color.RGBA{50, 60, 70, 255})}

	out, err := Generate(input, tiles, 10)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if out.Bounds().Dx() != 100 || out.Bounds().Dy() != 100 {
		t.Errorf("unexpected output bounds: %v", out.Bounds())
	}
}

func TestGenerate_Errors(t *testing.T) {
	validInput := solidImage(100, 100, color.RGBA{0, 0, 0, 255})
	validTiles := []image.Image{solidImage(10, 10, color.RGBA{0, 0, 0, 255})}

	tests := []struct {
		name     string
		input    image.Image
		tiles    []image.Image
		gridSize int
		wantErr  error
	}{
		{"no tiles", validInput, nil, 10, ErrNoTiles},
		{"empty tile slice", validInput, []image.Image{}, 10, ErrNoTiles},
		{"grid size below minimum", validInput, validTiles, 1, ErrGridSizeOutOfRange},
		{"grid size zero", validInput, validTiles, 0, ErrGridSizeOutOfRange},
		{"grid size negative", validInput, validTiles, -5, ErrGridSizeOutOfRange},
		{"grid size above maximum", validInput, validTiles, 301, ErrGridSizeOutOfRange},
		{"image too small for grid", solidImage(4, 4, color.RGBA{}), validTiles, 10, ErrImageTooSmall},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Generate(tt.input, tt.tiles, tt.gridSize)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("got error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerate_MaxGridSizeAccepted(t *testing.T) {
	// 300x300 at grid size 300 gives 1x1 cells: the finest valid configuration.
	input := solidImage(300, 300, color.RGBA{100, 100, 100, 255})
	tiles := []image.Image{solidImage(4, 4, color.RGBA{100, 100, 100, 255})}

	out, err := Generate(input, tiles, MaxGridSize)
	if err != nil {
		t.Fatalf("Generate returned error at max grid size: %v", err)
	}
	if out.Bounds().Dx() != 300 || out.Bounds().Dy() != 300 {
		t.Errorf("unexpected output bounds: %v", out.Bounds())
	}
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
