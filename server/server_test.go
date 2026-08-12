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
