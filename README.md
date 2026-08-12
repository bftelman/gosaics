# gosaics

Turn one photo into a mosaic built from many others.

gosaics splits your photo into a grid, then fills every cell with whichever image
from your collection best matches that cell's average colour. It ships as a single
binary that serves its own drag-and-drop web UI locally, so there is nothing to
install and nothing leaves your machine.

## Requirements

- Go 1.26 or newer (only to build; the resulting binary is self-contained)

## Run it

```bash
go run .
```

Or build a binary first:

```bash
go build -o gosaics .
./gosaics
```

gosaics starts on <http://localhost:8080> (the next free port if that one is
taken) and opens your browser automatically. If it can't, the URL is printed to
the terminal.

## Use it

1. Drop the photo you want to recreate into **Input photo**.
2. Drop a folder of photos into **Tile photos** — these become the mosaic's tiles.
3. Pick a **grid size**. Higher means more, smaller tiles and finer detail (and a
   longer wait). The default is 50; the range is 2–300.
4. Hit **Generate mosaic**, then download the result.

JPEG and PNG are supported for both the input photo and the tiles. Tiles are
reused as often as needed, so a small collection still works — a larger and more
colour-varied collection gives a better result.

## Development

```bash
go test ./... -race     # run the test suite
gofmt -l .              # check formatting
go vet ./...            # static analysis
```

The code is in three packages:

- `mosaic/` — the image algorithm (grid splitting, colour averaging, tile
  matching, compositing). No web dependencies, independently testable.
- `server/` — the HTTP layer: static asset serving and `POST /api/generate`.
- `web/` — the browser UI, embedded into the binary with `go:embed`.

## License

MIT — see [LICENSE](LICENSE).
