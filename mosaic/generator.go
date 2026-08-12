package mosaic

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"

	"golang.org/x/image/draw"
)

// Grid size bounds. The lower bound keeps the result recognizably a mosaic;
// the upper bound keeps memory and matching work sane.
const (
	MinGridSize     = 2
	MaxGridSize     = 300
	DefaultGridSize = 50
)

var (
	// ErrNoTiles means no usable tile images were supplied.
	ErrNoTiles = errors.New("no tile images provided")

	// ErrGridSizeOutOfRange means gridSize fell outside [MinGridSize, MaxGridSize].
	ErrGridSizeOutOfRange = fmt.Errorf("grid size must be between %d and %d", MinGridSize, MaxGridSize)

	// ErrImageTooSmall means the input image cannot be divided into the
	// requested grid without producing zero-sized cells.
	ErrImageTooSmall = errors.New("input image is too small for the requested grid size")
)

// Generate builds a photo mosaic of input out of the supplied tile images.
//
// The input is divided into a gridSize x gridSize grid. Each cell is matched to
// whichever tile has the closest average color, and that tile — stretched to the
// cell's dimensions — is drawn into the output. Tiles may be reused freely.
//
// The output is cellWidth*gridSize x cellHeight*gridSize, which can be slightly
// smaller than the input when its dimensions are not divisible by gridSize.
func Generate(input image.Image, tiles []image.Image, gridSize int) (image.Image, error) {
	if len(tiles) == 0 {
		return nil, ErrNoTiles
	}
	if gridSize < MinGridSize || gridSize > MaxGridSize {
		return nil, ErrGridSizeOutOfRange
	}

	cells := SplitGrid(input, gridSize)
	if cells == nil {
		return nil, ErrImageTooSmall
	}

	cellColors := averageColors(cells)
	tileColors := averageColors(tiles)

	matches := matchCellsToTiles(cellColors, tileColors)

	cellW, cellH := CellSize(input.Bounds(), gridSize)
	scaled := scaleMatchedTiles(tiles, matches, cellW, cellH)

	out := image.NewRGBA(image.Rect(0, 0, cellW*gridSize, cellH*gridSize))
	for i, tileIdx := range matches {
		row := i / gridSize
		col := i % gridSize
		dst := image.Rect(col*cellW, row*cellH, (col+1)*cellW, (row+1)*cellH)
		draw.Draw(out, dst, scaled[tileIdx], image.Point{}, draw.Src)
	}

	return out, nil
}

// averageColors computes the average color of every image concurrently.
// Each worker writes only its own index, so no locking is needed.
func averageColors(imgs []image.Image) []RGB {
	colors := make([]RGB, len(imgs))
	forEachIndex(len(imgs), func(i int) {
		colors[i] = AverageRGB(imgs[i])
	})
	return colors
}

// matchCellsToTiles returns, for each cell, the index of the nearest-colored
// tile. This is a brute-force scan over all tiles per cell, which is plenty
// fast at the sizes this tool allows.
func matchCellsToTiles(cellColors, tileColors []RGB) []int {
	matches := make([]int, len(cellColors))
	forEachIndex(len(cellColors), func(i int) {
		best := 0
		bestDist := SquaredDistance(cellColors[i], tileColors[0])
		for t := 1; t < len(tileColors); t++ {
			if d := SquaredDistance(cellColors[i], tileColors[t]); d < bestDist {
				best, bestDist = t, d
			}
		}
		matches[i] = best
	})
	return matches
}

// scaleMatchedTiles resizes each tile that was actually matched to cellW x cellH,
// keyed by tile index. Tiles that no cell selected are skipped, and each distinct
// matched tile is resized exactly once no matter how many cells use it.
func scaleMatchedTiles(tiles []image.Image, matches []int, cellW, cellH int) map[int]image.Image {
	used := make(map[int]struct{})
	for _, idx := range matches {
		used[idx] = struct{}{}
	}

	indices := make([]int, 0, len(used))
	for idx := range used {
		indices = append(indices, idx)
	}

	scaled := make([]image.Image, len(indices))
	forEachIndex(len(indices), func(i int) {
		dst := image.NewRGBA(image.Rect(0, 0, cellW, cellH))
		// Stretch to fill the cell, ignoring aspect ratio, so tiles cover the
		// canvas with no gaps.
		draw.CatmullRom.Scale(dst, dst.Bounds(), tiles[indices[i]], tiles[indices[i]].Bounds(), draw.Over, nil)
		scaled[i] = dst
	})

	byTileIndex := make(map[int]image.Image, len(indices))
	for i, idx := range indices {
		byTileIndex[idx] = scaled[i]
	}
	return byTileIndex
}

// forEachIndex runs fn for every index in [0, n) across a bounded pool of
// goroutines sized to the number of available CPUs.
func forEachIndex(n int, fn func(i int)) {
	if n == 0 {
		return
	}

	workers := runtime.NumCPU()
	if workers > n {
		workers = n
	}

	indices := make(chan int, n)
	for i := 0; i < n; i++ {
		indices <- i
	}
	close(indices)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := range indices {
				fn(i)
			}
		}()
	}
	wg.Wait()
}
