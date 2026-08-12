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

	for _, p := range parts {
		fw, err := mw.CreateFormFile(p.field, p.filename)
		if err != nil {
			t.Fatalf("creating form file %q: %v", p.filename, err)
		}
		if _, err := fw.Write(p.data); err != nil {
			t.Fatalf("writing form file %q: %v", p.filename, err)
		}
	}

	if gridSize != "" {
		if err := mw.WriteField("gridSize", gridSize); err != nil {
			t.Fatalf("writing gridSize field: %v", err)
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
		t.Errorf("got status %d, want 400", rec.Code)
	}
}
