package mosaic

import (
	"image"
	"image/color"
)

// solidImage returns a w x h image filled entirely with c.
func solidImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// halfAndHalfImage returns a w x h image whose left half is left and right half is right.
// w must be even for an exact 50/50 split.
func halfAndHalfImage(w, h int, left, right color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := left
			if x >= w/2 {
				c = right
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}
