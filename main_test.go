package main

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestListen_UsesBasePortWhenFree(t *testing.T) {
	// Ask the OS for a free port, release it, then confirm listen takes it.
	probe, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("probing for a free port: %v", err)
	}
	freePort := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	ln, err := listen(freePort, 5)
	if err != nil {
		t.Fatalf("listen returned error: %v", err)
	}
	defer ln.Close()

	if got := ln.Addr().(*net.TCPAddr).Port; got != freePort {
		t.Errorf("listening on port %d, want %d", got, freePort)
	}
}

func TestListen_FallsForwardWhenPortBusy(t *testing.T) {
	// Occupy a port, then confirm listen moves to the next one.
	busy, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("occupying a port: %v", err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port

	ln, err := listen(busyPort, 5)
	if err != nil {
		t.Fatalf("listen returned error: %v", err)
	}
	defer ln.Close()

	if got := ln.Addr().(*net.TCPAddr).Port; got == busyPort {
		t.Errorf("listen reused the busy port %d", got)
	}
}

func TestListen_ErrorsWhenNoPortAvailable(t *testing.T) {
	// Occupy a port and allow exactly one attempt, so there is no fallback.
	busy, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("occupying a port: %v", err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port

	ln, err := listen(busyPort, 1)
	if err == nil {
		ln.Close()
		t.Fatal("listen succeeded, want an error")
	}
	if !strings.Contains(err.Error(), fmt.Sprint(busyPort)) {
		t.Errorf("error %q does not mention the attempted port %d", err, busyPort)
	}
}

func TestListen_BindsLocalhostOnly(t *testing.T) {
	probe, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("probing for a free port: %v", err)
	}
	freePort := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	ln, err := listen(freePort, 5)
	if err != nil {
		t.Fatalf("listen returned error: %v", err)
	}
	defer ln.Close()

	// A localhost-bound listener must not be reachable on a wildcard address.
	ip := ln.Addr().(*net.TCPAddr).IP
	if !ip.IsLoopback() {
		t.Errorf("listening on %v, want a loopback address", ip)
	}
}
