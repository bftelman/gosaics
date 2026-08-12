// Package mosaic implements photo-mosaic generation: splitting a source image
// into a grid, matching each cell to the nearest-colored tile image, and
// compositing the matched tiles into a single output image.
package mosaic

import "image"

// RGB is an average color with channel values on the conventional 0-255 scale.
type RGB struct {
	R, G, B float64
}

// AverageRGB returns the mean color of every pixel in img.
// A zero-area image returns the zero RGB.
func AverageRGB(img image.Image) RGB {
	bounds := img.Bounds()
	count := bounds.Dx() * bounds.Dy()
	if count == 0 {
		return RGB{}
	}

	var sumR, sumG, sumB uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// RGBA returns 16-bit values; shift to the 0-255 scale.
			r, g, b, _ := img.At(x, y).RGBA()
			sumR += uint64(r >> 8)
			sumG += uint64(g >> 8)
			sumB += uint64(b >> 8)
		}
	}

	n := float64(count)
	return RGB{
		R: float64(sumR) / n,
		G: float64(sumG) / n,
		B: float64(sumB) / n,
	}
}

// SquaredDistance returns the squared Euclidean distance between two colors in
// RGB space. Squared distance is enough for nearest-color comparisons and
// avoids a square root per candidate.
func SquaredDistance(a, b RGB) float64 {
	dr := a.R - b.R
	dg := a.G - b.G
	db := a.B - b.B
	return dr*dr + dg*dg + db*db
}
