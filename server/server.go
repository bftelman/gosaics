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
		tracked := &responseTracker{ResponseWriter: w}

		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			// ErrAbortHandler is the stdlib's signal to drop the connection
			// silently. It has to keep travelling, not become a 500.
			if recovered == http.ErrAbortHandler {
				panic(recovered)
			}

			log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, recovered)

			// Once anything has been written the response is committed:
			// appending an error body here would corrupt what the client is
			// already reading, so logging is all that is left.
			if tracked.wrote {
				return
			}

			writeJSONError(tracked, http.StatusInternalServerError,
				"Something went wrong on the server. Please try again.")
		}()

		next.ServeHTTP(tracked, r)
	})
}

// responseTracker records whether the response has been committed yet, so
// panic recovery can tell a fresh response from one already in flight.
type responseTracker struct {
	http.ResponseWriter
	wrote bool
}

func (t *responseTracker) WriteHeader(status int) {
	t.wrote = true
	t.ResponseWriter.WriteHeader(status)
}

func (t *responseTracker) Write(b []byte) (int, error) {
	t.wrote = true
	return t.ResponseWriter.Write(b)
}
