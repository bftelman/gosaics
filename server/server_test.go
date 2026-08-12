package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew_ServesIndexAtRoot(t *testing.T) {
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "gosaics") {
		t.Error(`index response does not contain "gosaics"`)
	}
	if !strings.Contains(body, "app.js") {
		t.Error("index response does not reference app.js")
	}
}

func TestNew_ServesStaticAssets(t *testing.T) {
	tests := []struct {
		path        string
		wantContent string
	}{
		{"/styles.css", "--accent"},
		{"/app.js", "api/generate"},
		{"/strings.en.json", "app.title"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			New().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("got status %d, want 200", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.wantContent) {
				t.Errorf("%s does not contain %q", tt.path, tt.wantContent)
			}
		})
	}
}

func TestNew_UnknownAssetReturns404(t *testing.T) {
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/does-not-exist.css", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", rec.Code)
	}
}

func TestNew_RoutesGenerateEndpoint(t *testing.T) {
	// A GET to the generate route must reach the handler, which rejects
	// non-POST methods -- proving the route is wired rather than 404ing.
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/generate", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want 405", rec.Code)
	}
}

func TestRecoverPanic_ConvertsPanicTo500JSON(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	recoverPanic(panicking).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want 500", rec.Code)
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v (body: %s)", err, rec.Body.String())
	}
	if payload.Error == "" {
		t.Error(`response JSON has an empty "error" field`)
	}
}

func TestRecoverPanic_PassesThroughNormalResponses(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("fine"))
	})

	rec := httptest.NewRecorder()
	recoverPanic(ok).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("got status %d, want 418", rec.Code)
	}
	if rec.Body.String() != "fine" {
		t.Errorf("got body %q, want %q", rec.Body.String(), "fine")
	}
}

func TestRecoverPanic_DoesNotCorruptCommittedResponse(t *testing.T) {
	// A handler that panics mid-stream must leave the already-sent status and
	// body intact rather than having an error body appended to it.
	streaming := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial-image-bytes"))
		panic("boom mid-stream")
	})

	rec := httptest.NewRecorder()
	recoverPanic(streaming).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want the already-committed 200", rec.Code)
	}
	if body := rec.Body.String(); body != "partial-image-bytes" {
		t.Errorf("committed body was appended to: %q", body)
	}
}

func TestRecoverPanic_PropagatesErrAbortHandler(t *testing.T) {
	// ErrAbortHandler must keep propagating so the stdlib can drop the
	// connection silently instead of it becoming a 500.
	aborting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	})

	defer func() {
		if recovered := recover(); recovered != http.ErrAbortHandler {
			t.Errorf("got panic value %v, want http.ErrAbortHandler", recovered)
		}
	}()

	rec := httptest.NewRecorder()
	recoverPanic(aborting).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	t.Fatal("ServeHTTP returned normally, want the panic to propagate")
}
