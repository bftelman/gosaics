package server

import (
	"bytes"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestEndToEnd_GenerateThroughRealServer drives the full stack over HTTP:
// a real listener, the real router, multipart upload, and JPEG response.
func TestEndToEnd_GenerateThroughRealServer(t *testing.T) {
	srv := httptest.NewServer(New())
	defer srv.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	addFile := func(field, name string, data []byte) {
		fw, err := mw.CreateFormFile(field, name)
		if err != nil {
			t.Fatalf("creating form file %q: %v", name, err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatalf("writing form file %q: %v", name, err)
		}
	}

	// Input is half red, half blue; tiles cover both so matching has real choices.
	addFile("input", "photo.jpg", halfJPEG(t, 120, 120,
		color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}))
	addFile("tiles", "red.jpg", solidJPEG(t, 24, 24, color.RGBA{255, 0, 0, 255}))
	addFile("tiles", "blue.png", solidPNG(t, 24, 24, color.RGBA{0, 0, 255, 255}))
	addFile("tiles", "green.jpg", solidJPEG(t, 24, 24, color.RGBA{0, 255, 0, 255}))

	if err := mw.WriteField("gridSize", "12"); err != nil {
		t.Fatalf("writing gridSize: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	res, err := http.Post(srv.URL+"/api/generate", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("posting to /api/generate: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("got Content-Type %q, want image/jpeg", ct)
	}

	img, err := jpeg.Decode(res.Body)
	if err != nil {
		t.Fatalf("response body is not a valid JPEG: %v", err)
	}
	// 120 / 12 = 10 px cells, so the mosaic is exactly 120x120.
	if img.Bounds().Dx() != 120 || img.Bounds().Dy() != 120 {
		t.Errorf("mosaic bounds %v, want 120x120", img.Bounds())
	}
}

func TestEndToEnd_ServesUIThroughRealServer(t *testing.T) {
	srv := httptest.NewServer(New())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("getting index: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want 200", res.StatusCode)
	}
}
