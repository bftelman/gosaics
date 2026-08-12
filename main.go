// Command gosaics serves a local web UI for building photo mosaics.
package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/bftelman/gosaics/server"
)

const (
	basePort     = 8080
	portAttempts = 20
)

func main() {
	log.SetFlags(log.Ltime)

	ln, err := listen(basePort, portAttempts)
	if err != nil {
		log.Fatalf("gosaics: %v", err)
	}

	url := fmt.Sprintf("http://%s", ln.Addr().String())
	log.Printf("gosaics is running at %s (press Ctrl+C to stop)", url)

	// Opening the browser is a convenience; the URL above is always printed.
	go func() {
		// A brief pause lets Serve start accepting before the browser connects.
		time.Sleep(300 * time.Millisecond)
		if err := openBrowser(url); err != nil {
			log.Printf("could not open your browser automatically: %v", err)
		}
	}()

	srv := &http.Server{
		Handler:           server.New(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("gosaics: server stopped: %v", err)
	}
}

// listen binds a loopback listener, starting at basePort and moving forward one
// port at a time for up to attempts tries.
func listen(basePort, attempts int) (net.Listener, error) {
	var lastErr error

	for offset := 0; offset < attempts; offset++ {
		port := basePort + offset
		ln, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		if err == nil {
			return ln, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("no free port in %d-%d: %w", basePort, basePort+attempts-1, lastErr)
}

// openBrowser asks the operating system to open url in the default browser.
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}
