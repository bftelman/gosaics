// Package server exposes the HTTP layer of gosaics: it adapts multipart
// uploads from the browser into calls on the mosaic package.
package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bftelman/gosaics/mosaic"
	"golang.org/x/image/draw"
)

// maxInputBytes caps how large the input photo part is allowed to be.
const maxInputBytes = 64 << 20 // 64 MiB

// GenerateHandler handles POST /api/generate: it streams an input photo plus
// a set of tile photos from a multipart form and responds with the generated
// mosaic as JPEG bytes.
//
// The upload is read via a streaming multipart.Reader rather than
// ParseMultipartForm so that the number of tile parts is unbounded (see
// mime/multipart's 1000-part cap on ReadForm) and memory use does not grow
// with the size of the upload. Because a multipart stream cannot be rewound,
// the client MUST send "gridSize" and "input" before any "tiles" parts: the
// tile downscale limit is derived from the input photo's bounds and the grid
// size, and both must be known before the first tile is decoded.
func GenerateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "This endpoint only accepts POST requests.")
			return
		}

		start := time.Now()
		log.Printf("generate: request started")

		mr, err := r.MultipartReader()
		if err != nil {
			writeJSONError(w, http.StatusBadRequest,
				"Expected a file upload. Please use the gosaics page to submit your photos.")
			return
		}

		var (
			gridSize    int
			gridSizeSet bool // an explicit gridSize part has been read
			input       image.Image
			collector   *tileCollector
			tileIndex   int
		)

		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				writeJSONError(w, http.StatusBadRequest,
					"The upload was interrupted or malformed. Please try again.")
				return
			}

			switch part.FormName() {
			case "gridSize":
				if collector != nil {
					part.Close()
					writeJSONError(w, http.StatusBadRequest,
						"The grid size must be sent before the tile photos.")
					return
				}

				raw, err := io.ReadAll(io.LimitReader(part, 32))
				part.Close()
				if err != nil {
					writeJSONError(w, http.StatusBadRequest,
						"The upload was interrupted or malformed. Please try again.")
					return
				}

				size, err := parseGridSize(string(raw))
				if err != nil {
					writeJSONError(w, http.StatusBadRequest, err.Error())
					return
				}
				gridSize = size
				gridSizeSet = true

			case "input":
				if collector != nil {
					part.Close()
					writeJSONError(w, http.StatusBadRequest,
						"The input photo must be sent before the tile photos.")
					return
				}

				data, err := io.ReadAll(io.LimitReader(part, maxInputBytes+1))
				part.Close()
				if err != nil {
					writeJSONError(w, http.StatusBadRequest,
						"The upload was interrupted or malformed. Please try again.")
					return
				}
				if len(data) > maxInputBytes {
					writeJSONError(w, http.StatusBadRequest,
						"The input photo is too large. Please use a smaller photo.")
					return
				}

				img, err := mosaic.Decode(bytes.NewReader(data))
				if err != nil {
					log.Printf("generate: decoding input photo: %v", err)
					writeJSONError(w, http.StatusBadRequest,
						"The input photo could not be read. Please use a JPEG or PNG image.")
					return
				}
				input = img

			case "tiles":
				if input == nil {
					part.Close()
					writeJSONError(w, http.StatusBadRequest,
						"The tile photos arrived before the input photo. Please send the input photo first.")
					return
				}

				if collector == nil {
					if !gridSizeSet {
						gridSize = mosaic.DefaultGridSize
					}
					cellW, cellH := mosaic.CellSize(input.Bounds(), gridSize)
					sizeLimit := tileSizeLimit(cellW, cellH)
					inputBounds := input.Bounds()
					log.Printf("generate: input photo %dx%d, grid %d (%dx%d px cells, tiles capped at %d px)",
						inputBounds.Dx(), inputBounds.Dy(), gridSize, cellW, cellH, sizeLimit)
					collector = newTileCollector(sizeLimit, tileProgressInterval, start)
				}

				data, err := io.ReadAll(io.LimitReader(part, maxTileBytes+1))
				part.Close()
				if err != nil {
					log.Printf("generate: reading tile %q: %v", part.FileName(), err)
					collector.recordSkipped()
					continue
				}
				if len(data) > maxTileBytes {
					log.Printf("generate: skipping tile %q: exceeds %d byte limit", part.FileName(), maxTileBytes)
					collector.recordSkipped()
					continue
				}

				collector.submit(tileIndex, data)
				tileIndex++

			default:
				part.Close()
			}
		}

		if input == nil {
			writeJSONError(w, http.StatusBadRequest,
				"Please choose an input photo to turn into a mosaic.")
			return
		}

		var tiles []image.Image
		if collector != nil {
			tiles = collector.finish()
			decoded, skipped := collector.counts()
			if skipped > 0 {
				log.Printf("generate: decoded %d tiles in %s (%d skipped)",
					decoded, formatDuration(time.Since(start)), skipped)
			} else {
				log.Printf("generate: decoded %d tiles in %s", decoded, formatDuration(time.Since(start)))
			}
		}
		if len(tiles) == 0 {
			writeJSONError(w, http.StatusBadRequest,
				"No usable tile photos were found. Please drop a folder containing JPEG or PNG images.")
			return
		}

		log.Printf("generate: building mosaic from %d tiles into %d cells", len(tiles), gridSize*gridSize)

		result, err := mosaic.Generate(input, tiles, gridSize)
		if err != nil {
			switch {
			case errors.Is(err, mosaic.ErrImageTooSmall):
				writeJSONError(w, http.StatusBadRequest,
					"The input photo is too small for that grid size. Try a smaller grid size or a larger photo.")
			case errors.Is(err, mosaic.ErrGridSizeOutOfRange):
				writeJSONError(w, http.StatusBadRequest, gridSizeRangeMessage())
			case errors.Is(err, mosaic.ErrNoTiles):
				writeJSONError(w, http.StatusBadRequest,
					"No usable tile photos were found. Please drop a folder containing JPEG or PNG images.")
			default:
				log.Printf("generate: mosaic generation failed: %v", err)
				writeJSONError(w, http.StatusInternalServerError, "Mosaic generation failed. Please try again.")
			}
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-store")
		cw := &countingWriter{w: w}
		if err := mosaic.EncodeJPEG(cw, result); err != nil {
			// The status line is already sent, so this can only be logged.
			log.Printf("generate: encoding response: %v", err)
			return
		}

		resultBounds := result.Bounds()
		log.Printf("generate: done in %s, %dx%d mosaic, %s",
			formatDuration(time.Since(start)), resultBounds.Dx(), resultBounds.Dy(), formatBytes(cw.n))
	}
}

// countingWriter wraps an http.ResponseWriter to tally how many bytes are
// written through it, without buffering the response body: bytes are still
// passed straight through to the underlying writer.
type countingWriter struct {
	w http.ResponseWriter
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}

// formatDuration renders d as seconds with one decimal place (e.g. "58.2s"),
// which is far more readable in logs than Go's default Duration formatting.
func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// formatBytes renders n as a human-readable size (e.g. "2.1 MB").
func formatBytes(n int64) string {
	const kb = 1024
	const mb = kb * 1024
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// parseGridSize reads the gridSize form value, defaulting when it is absent.
func parseGridSize(raw string) (int, error) {
	if raw == "" {
		return mosaic.DefaultGridSize, nil
	}

	size, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New(gridSizeRangeMessage())
	}
	if size < mosaic.MinGridSize || size > mosaic.MaxGridSize {
		return 0, errors.New(gridSizeRangeMessage())
	}
	return size, nil
}

func gridSizeRangeMessage() string {
	return "Please choose a grid size between " +
		strconv.Itoa(mosaic.MinGridSize) + " and " + strconv.Itoa(mosaic.MaxGridSize) + "."
}

// maxTileBytes caps how large a single tile file is allowed to be. This
// guards against one absurd upload (an uncompressed TIFF, say) hogging
// memory or decode time; anything larger is skipped rather than read.
const maxTileBytes = 64 << 20 // 64 MiB

// tileProgressInterval controls how often tileCollector logs running decode
// progress: every tileProgressInterval completed tiles.
const tileProgressInterval = 100

// tileCollector decodes and downscales tiles concurrently across a worker
// pool sized to runtime.NumCPU, while preserving the order tiles were
// submitted in. It is fed raw tile bytes plus an index rather than open
// files or *multipart.FileHeader, so it can be reused by any caller that
// only has a stream of []byte (for example a streaming multipart reader)
// without change.
type tileCollector struct {
	sizeLimit int // max dimension passed to downscaleTile for each decoded tile
	interval  int // how often (in completed tiles) to log running progress
	start     time.Time

	sem chan struct{} // bounds how many decodes run at once (backpressure)
	wg  sync.WaitGroup

	completed atomic.Int64 // tiles finished (decoded or skipped), for progress logging
	skipped   atomic.Int64 // tiles that failed to decode, were oversized, or unreadable

	mu    sync.Mutex
	tiles map[int]image.Image // keyed by submitted index; failures leave gaps
}

// newTileCollector returns a tileCollector that downscales tiles to
// sizeLimit and decodes them across runtime.NumCPU workers, matching the
// worker-pool sizing mosaic.Generate already uses. It logs running decode
// progress every interval completed tiles, with elapsed times measured from
// start.
func newTileCollector(sizeLimit, interval int, start time.Time) *tileCollector {
	return &tileCollector{
		sizeLimit: sizeLimit,
		interval:  interval,
		start:     start,
		sem:       make(chan struct{}, runtime.NumCPU()),
		tiles:     make(map[int]image.Image),
	}
}

// submit decodes and downscales data on a pooled goroutine, storing the
// result under index for later retrieval by finish. It blocks once the pool
// is saturated, which bounds how many tiles' raw bytes and decoded images
// can be in flight at once regardless of how many tiles are submitted
// overall. A decode failure is logged and the index is simply omitted from
// the result; it is not fatal.
func (c *tileCollector) submit(index int, data []byte) {
	c.sem <- struct{}{}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() { <-c.sem }()

		img, err := mosaic.Decode(bytes.NewReader(data))
		if err != nil {
			log.Printf("generate: skipping tile at index %d: %v", index, err)
			c.skipped.Add(1)
			c.recordProgress()
			return
		}
		img = downscaleTile(img, c.sizeLimit)

		c.mu.Lock()
		c.tiles[index] = img
		c.mu.Unlock()
		c.recordProgress()
	}()
}

// recordSkipped counts a tile that never reached submit at all (for example
// one rejected for being oversized while still streaming in), so it is still
// reflected in the running progress count and the final skipped tally.
func (c *tileCollector) recordSkipped() {
	c.skipped.Add(1)
	c.recordProgress()
}

// recordProgress increments the completed-tile counter and, every interval
// completions, logs the running count and elapsed time since start. It is
// safe to call from multiple goroutines concurrently.
func (c *tileCollector) recordProgress() {
	n := c.completed.Add(1)
	if c.interval > 0 && n%int64(c.interval) == 0 {
		log.Printf("generate: decoded %d tiles (%s)", n, formatDuration(time.Since(c.start)))
	}
}

// counts returns the total number of tiles processed so far (decoded or
// skipped) and how many of those were skipped.
func (c *tileCollector) counts() (completed, skipped int64) {
	return c.completed.Load(), c.skipped.Load()
}

// finish waits for every submitted decode to complete and returns the
// surviving tiles ordered by their submitted index. Indices are not
// necessarily contiguous, since failed decodes leave gaps; those gaps are
// skipped rather than represented as nil images.
func (c *tileCollector) finish() []image.Image {
	c.wg.Wait()

	indices := make([]int, 0, len(c.tiles))
	for i := range c.tiles {
		indices = append(indices, i)
	}
	sort.Ints(indices)

	tiles := make([]image.Image, len(indices))
	for i, idx := range indices {
		tiles[i] = c.tiles[idx]
	}
	return tiles
}

// maxTileDimension caps how large a decoded tile is kept in memory. Tiles are
// only ever drawn at one grid cell's size, so anything beyond that is decoded
// and averaged for nothing. The ceiling keeps a coarse grid over a huge photo
// (few, very large cells) from pinning hundreds of full-resolution images in
// memory; at that point mild tile softening is the right trade.
const maxTileDimension = 512

// downscaleTile shrinks img so neither side exceeds limit, preserving aspect
// ratio. Images already within the limit are returned untouched.
func downscaleTile(img image.Image, limit int) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 || (w <= limit && h <= limit) {
		return img
	}

	scale := float64(limit) / float64(w)
	if h > w {
		scale = float64(limit) / float64(h)
	}

	dstW := max(1, int(float64(w)*scale))
	dstH := max(1, int(float64(h)*scale))

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
	return dst
}

// tileSizeLimit returns how large decoded tiles need to be kept for a mosaic
// with the given cell dimensions.
func tileSizeLimit(cellW, cellH int) int {
	limit := cellW
	if cellH > limit {
		limit = cellH
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxTileDimension {
		limit = maxTileDimension
	}
	return limit
}

// writeJSONError sends {"error": "..."} with the given status code.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("writing JSON error response: %v", err)
	}
}
