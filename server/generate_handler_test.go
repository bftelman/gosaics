package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bftelman/gosaics/mosaic"
)

// solidJPEG returns the JPEG bytes of a w x h image of a single color.
func solidJPEG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding JPEG fixture: %v", err)
	}
	return buf.Bytes()
}

// solidPNG returns the PNG bytes of a w x h image of a single color.
func solidPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding PNG fixture: %v", err)
	}
	return buf.Bytes()
}

// halfJPEG returns the JPEG bytes of a w x h image whose left half is left and
// right half is right.
func halfJPEG(t *testing.T, w, h int, left, right color.RGBA) []byte {
	t.Helper()
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
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding JPEG fixture: %v", err)
	}
	return buf.Bytes()
}

type filePart struct {
	field    string
	filename string
	data     []byte
}

// newMultipartRequest builds a POST request to /api/generate with the given
// file parts and optional gridSize form value.
func newMultipartRequest(t *testing.T, parts []filePart, gridSize string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// The server streams the upload and needs gridSize before any file part.
	if gridSize != "" {
		if err := mw.WriteField("gridSize", gridSize); err != nil {
			t.Fatalf("writing gridSize field: %v", err)
		}
	}

	for _, p := range parts {
		fw, err := mw.CreateFormFile(p.field, p.filename)
		if err != nil {
			t.Fatalf("creating form file %q: %v", p.filename, err)
		}
		if _, err := fw.Write(p.data); err != nil {
			t.Fatalf("writing form file %q: %v", p.filename, err)
		}
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/generate", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestGenerateHandler_HappyPath(t *testing.T) {
	parts := []filePart{
		{"input", "photo.jpg", solidJPEG(t, 100, 100, color.RGBA{255, 0, 0, 255})},
		{"tiles", "a.jpg", solidJPEG(t, 20, 20, color.RGBA{255, 0, 0, 255})},
		{"tiles", "b.png", solidPNG(t, 20, 20, color.RGBA{0, 0, 255, 255})},
	}
	req := newMultipartRequest(t, parts, "10")
	rec := httptest.NewRecorder()

	GenerateHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200. body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("got Content-Type %q, want image/jpeg", ct)
	}
	// The response must be a decodable JPEG of the expected size.
	got, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("response body is not a valid JPEG: %v", err)
	}
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 100 {
		t.Errorf("mosaic bounds %v, want 100x100", got.Bounds())
	}
}

func TestGenerateHandler_DefaultsGridSizeWhenAbsent(t *testing.T) {
	// A 100x100 input at the default grid size of 50 yields 2x2 px cells.
	parts := []filePart{
		{"input", "photo.jpg", solidJPEG(t, 100, 100, color.RGBA{10, 10, 10, 255})},
		{"tiles", "a.jpg", solidJPEG(t, 20, 20, color.RGBA{10, 10, 10, 255})},
	}
	req := newMultipartRequest(t, parts, "")
	rec := httptest.NewRecorder()

	GenerateHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200. body: %s", rec.Code, rec.Body.String())
	}
	got, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("response body is not a valid JPEG: %v", err)
	}
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 100 {
		t.Errorf("mosaic bounds %v, want 100x100", got.Bounds())
	}
}

func TestGenerateHandler_SkipsUndecodableTiles(t *testing.T) {
	// One junk tile among valid ones must be skipped, not fatal.
	parts := []filePart{
		{"input", "photo.jpg", solidJPEG(t, 100, 100, color.RGBA{0, 255, 0, 255})},
		{"tiles", "junk.txt", []byte("not an image at all")},
		{"tiles", "good.jpg", solidJPEG(t, 20, 20, color.RGBA{0, 255, 0, 255})},
	}
	req := newMultipartRequest(t, parts, "10")
	rec := httptest.NewRecorder()

	GenerateHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200. body: %s", rec.Code, rec.Body.String())
	}
}

func TestGenerateHandler_ValidationErrors(t *testing.T) {
	validInput := filePart{"input", "photo.jpg", solidJPEG(t, 100, 100, color.RGBA{0, 0, 0, 255})}
	validTile := filePart{"tiles", "a.jpg", solidJPEG(t, 20, 20, color.RGBA{0, 0, 0, 255})}

	tests := []struct {
		name        string
		parts       []filePart
		gridSize    string
		wantStatus  int
		wantErrPart string
	}{
		{
			name:        "missing input photo",
			parts:       []filePart{validTile},
			gridSize:    "10",
			wantStatus:  http.StatusBadRequest,
			wantErrPart: "input photo",
		},
		{
			name:        "no tiles at all",
			parts:       []filePart{validInput},
			gridSize:    "10",
			wantStatus:  http.StatusBadRequest,
			wantErrPart: "tile",
		},
		{
			name: "all tiles undecodable",
			parts: []filePart{
				validInput,
				{"tiles", "junk1.txt", []byte("nope")},
				{"tiles", "junk2.txt", []byte("also nope")},
			},
			gridSize:    "10",
			wantStatus:  http.StatusBadRequest,
			wantErrPart: "tile",
		},
		{
			name:        "input photo undecodable",
			parts:       []filePart{{"input", "photo.jpg", []byte("garbage")}, validTile},
			gridSize:    "10",
			wantStatus:  http.StatusBadRequest,
			wantErrPart: "input photo",
		},
		{
			name:        "grid size not a number",
			parts:       []filePart{validInput, validTile},
			gridSize:    "abc",
			wantStatus:  http.StatusBadRequest,
			wantErrPart: "grid size",
		},
		{
			name:        "grid size below minimum",
			parts:       []filePart{validInput, validTile},
			gridSize:    "1",
			wantStatus:  http.StatusBadRequest,
			wantErrPart: "grid size",
		},
		{
			name:        "grid size above maximum",
			parts:       []filePart{validInput, validTile},
			gridSize:    "301",
			wantStatus:  http.StatusBadRequest,
			wantErrPart: "grid size",
		},
		{
			name:        "input too small for grid",
			parts:       []filePart{{"input", "tiny.jpg", solidJPEG(t, 4, 4, color.RGBA{0, 0, 0, 255})}, validTile},
			gridSize:    "50",
			wantStatus:  http.StatusBadRequest,
			wantErrPart: "too small",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newMultipartRequest(t, tt.parts, tt.gridSize)
			rec := httptest.NewRecorder()

			GenerateHandler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("got status %d, want %d. body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			var payload struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("response is not JSON: %v (body: %s)", err, rec.Body.String())
			}
			if payload.Error == "" {
				t.Fatal(`response JSON has an empty "error" field`)
			}
			if !strings.Contains(strings.ToLower(payload.Error), tt.wantErrPart) {
				t.Errorf("error %q does not mention %q", payload.Error, tt.wantErrPart)
			}
		})
	}
}

func TestGenerateHandler_RejectsNonPOST(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/generate", nil)
			rec := httptest.NewRecorder()

			GenerateHandler().ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("got status %d, want 405", rec.Code)
			}
		})
	}
}

func TestGenerateHandler_RejectsNonMultipartBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/generate", strings.NewReader("plain text"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	GenerateHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v (body: %s)", err, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(payload.Error), "file upload") {
		t.Errorf("error %q does not mention expecting a file upload", payload.Error)
	}
}

func TestGenerateHandler_AcceptsMoreThanAThousandTiles(t *testing.T) {
	// Regression guard: ParseMultipartForm capped a form at 1000 parts, so
	// 999+ tiles used to fail with "multipart: message too large". Streaming
	// has no such ceiling. Tiles are 1x1 to keep the test fast.
	parts := []filePart{
		{"input", "photo.jpg", solidJPEG(t, 100, 100, color.RGBA{90, 90, 90, 255})},
	}
	for i := 0; i < 1005; i++ {
		v := uint8(i % 256)
		parts = append(parts, filePart{"tiles", "t.png", solidPNG(t, 1, 1, color.RGBA{v, v, v, 255})})
	}

	req := newMultipartRequest(t, parts, "10")
	rec := httptest.NewRecorder()

	GenerateHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d with 1005 tiles, want 200. body: %s", rec.Code, rec.Body.String())
	}
	if got, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes())); err != nil {
		t.Fatalf("response is not a valid JPEG: %v", err)
	} else if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 100 {
		t.Errorf("mosaic bounds %v, want 100x100", got.Bounds())
	}
}

func TestGenerateHandler_TilesBeforeInputRejected(t *testing.T) {
	// The client is required to send the input photo before any tile photo,
	// since the tile downscale limit depends on the input's bounds. Sending
	// a tile first must fail with a message about the ordering, not a
	// confusing decode error.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	fw, err := mw.CreateFormFile("tiles", "a.jpg")
	if err != nil {
		t.Fatalf("creating tiles part: %v", err)
	}
	if _, err := fw.Write(solidJPEG(t, 20, 20, color.RGBA{0, 0, 0, 255})); err != nil {
		t.Fatalf("writing tiles part: %v", err)
	}

	iw, err := mw.CreateFormFile("input", "photo.jpg")
	if err != nil {
		t.Fatalf("creating input part: %v", err)
	}
	if _, err := iw.Write(solidJPEG(t, 100, 100, color.RGBA{0, 0, 0, 255})); err != nil {
		t.Fatalf("writing input part: %v", err)
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/generate", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	GenerateHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400. body: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v (body: %s)", err, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(payload.Error), "input photo") {
		t.Errorf("error %q does not mention the input photo needing to come first", payload.Error)
	}
}

func TestGenerateHandler_GridSizeAfterTilesRejected(t *testing.T) {
	// Once the first tile has been read, the tile downscale limit is already
	// locked in from the grid size seen so far, so a later gridSize part must
	// be rejected with a message explaining it must come first.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	if err := mw.WriteField("gridSize", "10"); err != nil {
		t.Fatalf("writing gridSize field: %v", err)
	}

	iw, err := mw.CreateFormFile("input", "photo.jpg")
	if err != nil {
		t.Fatalf("creating input part: %v", err)
	}
	if _, err := iw.Write(solidJPEG(t, 100, 100, color.RGBA{0, 0, 0, 255})); err != nil {
		t.Fatalf("writing input part: %v", err)
	}

	fw, err := mw.CreateFormFile("tiles", "a.jpg")
	if err != nil {
		t.Fatalf("creating tiles part: %v", err)
	}
	if _, err := fw.Write(solidJPEG(t, 20, 20, color.RGBA{0, 0, 0, 255})); err != nil {
		t.Fatalf("writing tiles part: %v", err)
	}

	if err := mw.WriteField("gridSize", "20"); err != nil {
		t.Fatalf("writing second gridSize field: %v", err)
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/generate", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	GenerateHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400. body: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v (body: %s)", err, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(payload.Error), "grid size") {
		t.Errorf("error %q does not mention the grid size needing to come first", payload.Error)
	}
}

func TestDownscaleTile(t *testing.T) {
	tests := []struct {
		name         string
		w, h, limit  int
		wantW, wantH int
	}{
		{"already small is untouched", 40, 30, 512, 40, 30},
		{"exactly at limit is untouched", 64, 64, 64, 64, 64},
		{"landscape shrinks by width", 1000, 500, 100, 100, 50},
		{"portrait shrinks by height", 500, 1000, 100, 50, 100},
		{"square shrinks evenly", 800, 800, 200, 200, 200},
		{"extreme ratio keeps at least one pixel", 5000, 2, 100, 100, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := image.NewRGBA(image.Rect(0, 0, tt.w, tt.h))
			got := downscaleTile(src, tt.limit).Bounds()
			if got.Dx() != tt.wantW || got.Dy() != tt.wantH {
				t.Errorf("got %dx%d, want %dx%d", got.Dx(), got.Dy(), tt.wantW, tt.wantH)
			}
		})
	}
}

func TestTileSizeLimit(t *testing.T) {
	tests := []struct {
		name         string
		cellW, cellH int
		want         int
	}{
		{"uses the larger side", 40, 25, 40},
		{"uses the larger side when tall", 25, 40, 40},
		{"clamped to the ceiling", 4000, 4000, maxTileDimension},
		{"never below one", 0, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tileSizeLimit(tt.cellW, tt.cellH); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGenerateHandler_LargeTilesDoNotChangeOutput(t *testing.T) {
	// A deliberately oversized tile must still produce a correct mosaic: the
	// handler downscales it on the way in, and the output geometry is driven
	// by the input photo and grid size, never by tile resolution.
	parts := []filePart{
		{"input", "photo.jpg", solidJPEG(t, 100, 100, color.RGBA{10, 200, 90, 255})},
		{"tiles", "huge.jpg", solidJPEG(t, 1600, 1200, color.RGBA{10, 200, 90, 255})},
	}
	req := newMultipartRequest(t, parts, "10")
	rec := httptest.NewRecorder()

	GenerateHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200. body: %s", rec.Code, rec.Body.String())
	}

	got, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("response is not a valid JPEG: %v", err)
	}
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 100 {
		t.Errorf("mosaic bounds %v, want 100x100", got.Bounds())
	}
}

func TestTileCollector_PreservesOrder(t *testing.T) {
	// Tile order decides tie-breaks in nearest-colour matching, so a
	// concurrent decoder must still hand tiles back in submission order.
	const n = 40
	collector := newTileCollector(64)
	for i := 0; i < n; i++ {
		// Each tile is a distinct, recognisable grey so position is checkable.
		v := uint8(i * 5)
		collector.submit(i, solidPNG(t, 8, 8, color.RGBA{v, v, v, 255}))
	}

	tiles := collector.finish()
	if len(tiles) != n {
		t.Fatalf("got %d tiles, want %d", len(tiles), n)
	}

	for i, tile := range tiles {
		got := mosaic.AverageRGB(tile)
		want := float64(uint8(i * 5))
		if absDiffF(got.R, want) > 2 {
			t.Errorf("tile %d has average red %.1f, want ~%.0f (order not preserved)", i, got.R, want)
		}
	}
}

func TestTileCollector_SkipsFailuresAndKeepsOrder(t *testing.T) {
	// A broken tile in the middle must be dropped without shifting the
	// relative order of the tiles around it.
	collector := newTileCollector(64)
	collector.submit(0, solidPNG(t, 8, 8, color.RGBA{10, 10, 10, 255}))
	collector.submit(1, []byte("not an image"))
	collector.submit(2, solidPNG(t, 8, 8, color.RGBA{200, 200, 200, 255}))

	tiles := collector.finish()
	if len(tiles) != 2 {
		t.Fatalf("got %d tiles, want 2 (the broken one skipped)", len(tiles))
	}
	if first := mosaic.AverageRGB(tiles[0]); absDiffF(first.R, 10) > 2 {
		t.Errorf("first surviving tile has average red %.1f, want ~10", first.R)
	}
	if second := mosaic.AverageRGB(tiles[1]); absDiffF(second.R, 200) > 2 {
		t.Errorf("second surviving tile has average red %.1f, want ~200", second.R)
	}
}

func absDiffF(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
