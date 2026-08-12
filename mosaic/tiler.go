package mosaic

import "image"

// CellSize returns the pixel dimensions of a single grid cell for the given
// image bounds and grid size. Integer division means a source whose dimensions
// are not divisible by gridSize leaves a few edge pixels unused, which keeps
// every cell exactly the same size.
func CellSize(bounds image.Rectangle, gridSize int) (w, h int) {
	if gridSize <= 0 {
		return 0, 0
	}
	return bounds.Dx() / gridSize, bounds.Dy() / gridSize
}

// SplitGrid divides img into a gridSize x gridSize grid of equally sized cells,
// returned in row-major order: index i is row i/gridSize, column i%gridSize.
//
// It returns nil if gridSize is not positive, or if the grid is so fine that
// cells would have zero width or height.
func SplitGrid(img image.Image, gridSize int) []image.Image {
	if gridSize <= 0 {
		return nil
	}

	bounds := img.Bounds()
	cellW, cellH := CellSize(bounds, gridSize)
	if cellW == 0 || cellH == 0 {
		return nil
	}

	cells := make([]image.Image, 0, gridSize*gridSize)
	for row := 0; row < gridSize; row++ {
		for col := 0; col < gridSize; col++ {
			minX := bounds.Min.X + col*cellW
			minY := bounds.Min.Y + row*cellH
			rect := image.Rect(minX, minY, minX+cellW, minY+cellH)

			// SubImage shares pixels with img rather than copying them, so
			// splitting a large source stays cheap.
			if sub, ok := img.(interface {
				SubImage(r image.Rectangle) image.Image
			}); ok {
				cells = append(cells, sub.SubImage(rect))
				continue
			}
			cells = append(cells, cropCopy(img, rect))
		}
	}

	return cells
}

// cropCopy copies the pixels of img inside rect into a new image. It is the
// fallback for image types that do not implement SubImage.
func cropCopy(img image.Image, rect image.Rectangle) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			out.Set(x, y, img.At(rect.Min.X+x, rect.Min.Y+y))
		}
	}
	return out
}
