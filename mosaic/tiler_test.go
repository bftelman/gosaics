package mosaic

import (
	"image"
	"image/color"
	"testing"
)

func TestCellSize(t *testing.T) {
	tests := []struct {
		name           string
		w, h, gridSize int
		wantW, wantH   int
	}{
		{"evenly divisible square", 100, 100, 10, 10, 10},
		{"evenly divisible rectangle", 200, 100, 10, 20, 10},
		{"grid size 1 returns whole image", 37, 53, 1, 37, 53},
		{"non-divisible truncates", 105, 105, 10, 10, 10},
		{"grid larger than image clamps to zero", 5, 5, 10, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := CellSize(image.Rect(0, 0, tt.w, tt.h), tt.gridSize)
			if gotW != tt.wantW || gotH != tt.wantH {
				t.Errorf("CellSize(%dx%d, %d) = %dx%d, want %dx%d",
					tt.w, tt.h, tt.gridSize, gotW, gotH, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestSplitGrid_CellCount(t *testing.T) {
	tests := []struct {
		name      string
		gridSize  int
		wantCells int
	}{
		{"grid size 1", 1, 1},
		{"grid size 2", 2, 4},
		{"grid size 5", 5, 25},
		{"grid size 10", 10, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := solidImage(40, 40, color.RGBA{1, 2, 3, 255})
			cells := SplitGrid(img, tt.gridSize)
			if len(cells) != tt.wantCells {
				t.Errorf("got %d cells, want %d", len(cells), tt.wantCells)
			}
		})
	}
}

func TestSplitGrid_CellDimensions(t *testing.T) {
	// 60x40 split 4 ways -> each cell 15x10.
	img := solidImage(60, 40, color.RGBA{0, 0, 0, 255})
	cells := SplitGrid(img, 4)

	for i, cell := range cells {
		b := cell.Bounds()
		if b.Dx() != 15 || b.Dy() != 10 {
			t.Errorf("cell %d is %dx%d, want 15x10", i, b.Dx(), b.Dy())
		}
	}
}

func TestSplitGrid_RowMajorOrderAndContent(t *testing.T) {
	// Left half red, right half blue, split into a 2x2 grid.
	// Row-major: indices 0,2 are the left column (red); 1,3 the right column (blue).
	red := color.RGBA{255, 0, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}
	img := halfAndHalfImage(20, 20, red, blue)

	cells := SplitGrid(img, 2)
	if len(cells) != 4 {
		t.Fatalf("got %d cells, want 4", len(cells))
	}

	wants := []RGB{
		{255, 0, 0}, // row 0, col 0 -> red
		{0, 0, 255}, // row 0, col 1 -> blue
		{255, 0, 0}, // row 1, col 0 -> red
		{0, 0, 255}, // row 1, col 1 -> blue
	}
	for i, want := range wants {
		assertRGBNear(t, AverageRGB(cells[i]), want)
	}
}

func TestSplitGrid_NonDivisibleDimensions(t *testing.T) {
	// 105x105 into a 10x10 grid -> 100 cells of 10x10; the last 5 px/axis are dropped.
	img := solidImage(105, 105, color.RGBA{9, 9, 9, 255})
	cells := SplitGrid(img, 10)

	if len(cells) != 100 {
		t.Fatalf("got %d cells, want 100", len(cells))
	}
	for i, cell := range cells {
		b := cell.Bounds()
		if b.Dx() != 10 || b.Dy() != 10 {
			t.Errorf("cell %d is %dx%d, want 10x10", i, b.Dx(), b.Dy())
		}
	}
}

func TestSplitGrid_InvalidInputsReturnNil(t *testing.T) {
	img := solidImage(20, 20, color.RGBA{0, 0, 0, 255})

	if cells := SplitGrid(img, 0); cells != nil {
		t.Errorf("gridSize 0: got %d cells, want nil", len(cells))
	}
	if cells := SplitGrid(img, -3); cells != nil {
		t.Errorf("negative gridSize: got %d cells, want nil", len(cells))
	}
	// Grid finer than the image has zero-sized cells -> nil.
	if cells := SplitGrid(solidImage(5, 5, color.RGBA{}), 10); cells != nil {
		t.Errorf("oversized grid: got %d cells, want nil", len(cells))
	}
}
