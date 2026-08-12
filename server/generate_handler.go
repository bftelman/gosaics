// Package server exposes the HTTP layer of gosaics: it adapts multipart
// uploads from the browser into calls on the mosaic package.
package server

import (
	"encoding/json"
	"errors"
	"image"
	"log"
	"net/http"
	"strconv"

	"github.com/bftelman/gosaics/mosaic"
)

// maxUploadBytes bounds how much of a multipart upload is buffered in memory
// before overflow spills to temporary files. A tile folder can be large, so
// this is generous.
const maxUploadBytes = 512 << 20 // 512 MiB

// GenerateHandler handles POST /api/generate: it reads an input photo plus a
// set of tile photos from a multipart form and responds with the generated
// mosaic as JPEG bytes.
func GenerateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "This endpoint only accepts POST requests.")
			return
		}

		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			log.Printf("generate: parsing multipart form: %v", err)
			writeJSONError(w, http.StatusBadRequest, "Could not read the uploaded files. Please try again.")
			return
		}
		defer func() {
			if r.MultipartForm != nil {
				// Removes any temporary files the form spilled to disk.
				_ = r.MultipartForm.RemoveAll()
			}
		}()

		gridSize, err := parseGridSize(r.FormValue("gridSize"))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		input, err := decodeInputPhoto(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		tiles := decodeTiles(r)
		if len(tiles) == 0 {
			writeJSONError(w, http.StatusBadRequest,
				"No usable tile photos were found. Please drop a folder containing JPEG or PNG images.")
			return
		}

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
		if err := mosaic.EncodeJPEG(w, result); err != nil {
			// The status line is already sent, so this can only be logged.
			log.Printf("generate: encoding response: %v", err)
		}
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

// decodeInputPhoto reads and decodes the single "input" file part.
func decodeInputPhoto(r *http.Request) (image.Image, error) {
	file, header, err := r.FormFile("input")
	if err != nil {
		return nil, errors.New("Please choose an input photo to turn into a mosaic.")
	}
	defer file.Close()

	img, err := mosaic.Decode(file)
	if err != nil {
		log.Printf("generate: decoding input photo %q: %v", header.Filename, err)
		return nil, errors.New("The input photo could not be read. Please use a JPEG or PNG image.")
	}
	return img, nil
}

// decodeTiles decodes every "tiles" file part, skipping any that fail. A single
// unreadable file among many should not fail the whole request.
func decodeTiles(r *http.Request) []image.Image {
	if r.MultipartForm == nil {
		return nil
	}

	headers := r.MultipartForm.File["tiles"]
	tiles := make([]image.Image, 0, len(headers))

	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			log.Printf("generate: opening tile %q: %v", header.Filename, err)
			continue
		}

		img, err := mosaic.Decode(file)
		file.Close()
		if err != nil {
			log.Printf("generate: skipping tile %q: %v", header.Filename, err)
			continue
		}
		tiles = append(tiles, img)
	}

	return tiles
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
