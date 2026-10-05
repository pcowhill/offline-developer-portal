package main

import (
	"net"
	"path/filepath"
	"testing"
)

func TestListenFallsBackToNextFreePort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port

	ln, err := listen("127.0.0.1", port, true)
	if err != nil {
		t.Fatalf("expected fallback, got %v", err)
	}
	defer ln.Close()
	if got := ln.Addr().(*net.TCPAddr).Port; got == port {
		t.Fatalf("listened on busy port %d", got)
	}
	if _, err := listen("127.0.0.1", port, false); err == nil {
		t.Fatal("an explicitly requested busy port must be an error")
	}
}

func TestValidateExampleConfig(t *testing.T) {
	if err := runValidate([]string{"--config", filepath.Join("..", "..", "sources.example.yaml")}); err != nil {
		t.Fatal(err)
	}
	if err := runValidate([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml")}); err == nil {
		t.Fatal("expected error for a missing file")
	}
}

func TestServeRequiresDist(t *testing.T) {
	if err := runServe([]string{"--dist", t.TempDir(), "--port", "0"}); err == nil {
		t.Fatal("expected an error when dist/index.html is missing")
	}
}
