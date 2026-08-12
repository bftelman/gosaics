# gosaics — design spec

Date: 2026-08-12

## Summary

`gosaics` is a Go rewrite of [`mosaics`](https://gitlab.com/upperlimit/mosaics) (a .NET CLI photo-mosaic
generator). Instead of an interactive terminal prompt, `gosaics` is a single self-contained Go binary
that starts a local web server, opens the browser, and provides a drag-and-drop UI for generating photo
mosaics: drop one input photo and a folder of tile photos, pick a grid size, and get a mosaic built from
the tile images, best-matched by average color per grid cell.

## Goals

- One binary, no runtime dependencies (no Node, no ImageMagick, no database).
- Minimal, clean, "cool" web UI: drag-and-drop input photo + tile folder, grid size control, language
  selector, dark/light toggle, help.
- Same core mosaic algorithm as `mosaics`: split input image into an N×N grid, compute each cell's
  average RGB, compute each tile image's average RGB, match each cell to its nearest-color tile
  (squared RGB distance), composite the result.
- Fast: use goroutines to parallelize color computation and matching, matching the spirit of the
  original's `Parallel.For`/`Parallel.ForEach` usage.

## Non-goals

- No CLI mode (web UI only).
- No persistence/database — everything is in-memory, per-request.
- No account system, no multi-user concerns — this is a local single-user tool.
- No WebP/GIF support (JPEG + PNG only, matching what Go's stdlib supports natively).
- No live progress percentage — a spinner is sufficient; no job-tracking/SSE infrastructure.
- No tile-reuse avoidance — tiles may be reused across cells, exactly like the original.

## Architecture

Single Go module, `github.com/bftelman/gosaics`. On startup:

1. Start an `net/http` server on `localhost:8080` (probe and increment the port if already in use).
2. Open the user's default browser to that address (`xdg-open`/`open`/`start` per-OS, best-effort — if it
   fails, just print the URL).
3. Serve the embedded web UI and handle mosaic-generation requests.

No external image library — Go's standard `image`, `image/jpeg`, and `image/png` packages cover decode,
resize (via simple nearest-neighbor or `draw.CatmullRom`-style scaling written locally, since stdlib has
no resize primitive beyond `draw.Draw`/`x/image/draw`), and encode.

Decision: use `golang.org/x/image/draw` for high-quality resampling (it's the standard extended library,
not a third-party dependency in spirit — maintained by the Go team). This avoids hand-rolling resampling
math and keeps output quality reasonable.

## Package layout

```
gosaics/
  go.mod
  main.go                  # entrypoint: start server, open browser
  mosaic/
    tiler.go                # grid-splitting of the input image
    tiler_test.go
    color.go                 # average RGB, squared color distance
    color_test.go
    generator.go             # orchestrates tiling + matching + compositing
    generator_test.go
  server/
    server.go                # http.Handler setup, routes
    generate_handler.go       # POST /api/generate
    generate_handler_test.go
  web/
    assets.go                 # //go:embed directive
    index.html
    app.js
    styles.css
    strings.en.json
  docs/
    superpowers/specs/...
  .gitignore
  LICENSE
  README.md
```

`mosaic/` has zero HTTP/web knowledge and is fully unit-testable on its own. `server/` depends on
`mosaic/` and adapts HTTP requests to it. `web/` is embedded static content plus the JSON string table.

## Core algorithm (mosaic package)

Direct port of the `mosaics` logic:

- `SplitGrid(img image.Image, gridSize int) [][]image.Image` — crops the input image into
  `gridSize × gridSize` sub-images (row-major), same integer division approach as the C# version
  (`x*inWidth/size`, `y*inHeight/size`).
- `AverageRGB(img image.Image) RGB` — mean R/G/B over all pixels (equivalent to ImageMagick's channel
  mean statistic).
- `SquaredDistance(a, b RGB) float64` — sum of squared per-channel differences.
- `Generate(input image.Image, tiles []image.Image, gridSize int) (image.Image, error)`:
  1. Compute grid cells and their average colors (parallel, worker pool sized to `runtime.NumCPU()`).
  2. Compute each tile's average color (parallel, same pool pattern).
  3. For each grid cell, linear-scan all tile colors and pick the minimum squared-distance tile
     (mirrors the original's O(cells × tiles) brute-force approach — fine at these scales; no need for a
     k-d tree given YAGNI and the original didn't use one either).
  4. Resize (ignore aspect ratio, matching the original) each matched tile to the cell's pixel
     dimensions (`inputWidth/gridSize`, `inputHeight/gridSize`) via `x/image/draw`, and composite into
     an output canvas of size `cellWidth*gridSize × cellHeight*gridSize`.
  5. Return the composited image.

Concurrency: a simple bounded worker pool (buffered channel of work indices + `sync.WaitGroup`), sized to
`runtime.NumCPU()`, used for both the color-averaging passes and the per-cell matching pass — no need for
anything fancier.

## HTTP API

### `POST /api/generate`

Multipart form:
- `input` — one file field, the input photo (jpeg/png)
- `tiles` — repeated file field, the tile photos (jpeg/png), from a dropped folder
- `gridSize` — form field, integer, 2–300

Response: `200 OK`, `Content-Type: image/jpeg`, body = the generated mosaic.

Errors: `400 Bad Request` with JSON body `{"error": "<human-readable message>"}` for:
- missing input photo
- zero usable (jpeg/png) tile files after filtering unsupported types
- grid size outside 2–300 or non-integer
- input photo fails to decode

Unsupported files among the tiles are silently skipped (not a hard error) as long as at least one usable
tile remains — mirrors the original's `*.jpg` filtering.

`500 Internal Server Error` with the same JSON error shape for unexpected failures, logged server-side
with details.

## Frontend

Plain HTML/CSS/JS, no framework, no build step — embedded directly.

- Two drop zones: "Input Photo" (single file) and "Tile Photos" (folder or multiple files). Folder drop
  via `DataTransferItem.webkitGetAsEntry()` recursively collecting files; a fallback "Browse folder"
  `<input type="file" webkitdirectory multiple>` button for browsers/cases where drag-drop of a folder
  doesn't fire entries (also serves click-to-browse).
- Grid size: numeric input, default 50, clamped client-side to 2–300 before submit.
- Generate button: disabled until both an input photo and at least one tile file are present.
- On submit: build `FormData`, `fetch('/api/generate', {method: 'POST', body: form})`, show a spinner
  with rotating status text (e.g. "Matching tiles...", "Compositing..." — cosmetic only, no real progress
  tie-in). On success, render the returned blob in an `<img>` and show a "Download mosaic" link
  (`download` attribute). On failure, show the error message inline near the Generate button.
- Language selector: `<select>` in the header driving a `strings.en.json`-backed lookup used to populate
  all UI text via a small `t(key)` helper in `app.js`. Only `en` populated now; adding a language later is
  just dropping in another `strings.<code>.json` and an option in the select.
- Dark/light toggle: CSS custom properties for colors, `prefers-color-scheme` as the default, a toggle
  button that sets `data-theme` on `<html>` and persists the choice to `localStorage`.
- Help: a "?" icon button opening a simple modal (native `<dialog>`) with 3-4 lines explaining the two
  drop zones, grid size, and the generate/download flow.

## Error handling (cross-cutting)

- Server validates all inputs before doing any decode/processing work, returning clear 400s.
- Decode failures on individual tile files are skipped with a server-side log line, not a fatal error
  (a corrupt/unsupported tile shouldn't kill the whole request).
- Decode failure on the input photo is fatal (400) since there's nothing to mosaic.
- All unexpected panics in the handler are recovered via middleware, logged, and converted to a 500 JSON
  error rather than crashing the server process.
- Frontend never leaves the user stuck on the spinner: both success and failure paths always end the
  loading state.

## Testing

- `mosaic/color_test.go` — table-driven tests for `AverageRGB` on synthetic images (solid colors, known
  gradients) and `SquaredDistance` on known color pairs.
- `mosaic/tiler_test.go` — table-driven tests asserting grid cell count and pixel bounds for various
  image dimensions and grid sizes, including edge cases (grid size 1, non-divisible dimensions).
- `mosaic/generator_test.go` — end-to-end test of `Generate` using small synthetically-generated images
  (solid-color squares as both input and tiles, built in-memory in the test — no binary fixtures checked
  into the repo), asserting output dimensions and that no error occurs.
- `server/generate_handler_test.go` — `httptest`-based table-driven tests covering the validation error
  paths (missing input, no usable tiles, bad grid size) and one happy-path test using small in-memory
  generated images as multipart parts, asserting `200` + `image/jpeg` content type.
- No frontend test framework introduced — the JS is simple enough to be covered by manual verification
  (YAGNI); can be revisited if the UI grows more complex.

## Repository

- `github.com/bftelman/gosaics`, public, MIT license.
- Standard `go.mod`, Go 1.26.
- `.gitignore` for build artifacts (`/gosaics`, `/bin/`, etc.) and OS/editor cruft.
- `README.md` covering: what it is, how to build/run (`go run .` or `go build`), a short usage
  walkthrough, and a credit line back to the original `mosaics` project.
