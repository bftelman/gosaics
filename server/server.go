package server

import (
	"log"
	"net/http"

	"github.com/bftelman/gosaics/web"
)

// New returns the complete gosaics HTTP handler: the embedded browser UI plus
// the mosaic generation endpoint, wrapped in panic recovery.
func New() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/api/generate", GenerateHandler())

	// Everything else is a static asset from the embedded filesystem; "/"
	// resolves to index.html.
	mux.Handle("/", http.FileServer(http.FS(web.FS)))

	return recoverPanic(mux)
}

// recoverPanic turns a panic in any downstream handler into a logged 500
// instead of tearing down the whole process.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, recovered)
				writeJSONError(w, http.StatusInternalServerError,
					"Something went wrong on the server. Please try again.")
			}
		}()

		next.ServeHTTP(w, r)
	})
}
