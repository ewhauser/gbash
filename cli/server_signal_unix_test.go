//go:build unix

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ewhauser/gbash"
)

//nolint:paralleltest // sends SIGTERM to the test process
func TestServerStopsOnSIGTERMAndRemovesSocket(t *testing.T) {
	rt, err := gbash.New()
	if err != nil {
		t.Fatalf("gbash.New() error = %v", err)
	}
	dir, err := os.MkdirTemp("", "gbs")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s.sock")

	done := make(chan error, 1)
	go func() {
		done <- serveServer(context.Background(), rt, "gbash", "test", &runtimeOptions{socket: socket})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server socket never appeared")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	select {
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "closed") {
			t.Fatalf("serveServer() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop on SIGTERM")
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Fatalf("socket still present after SIGTERM (stat err = %v)", err)
	}
}
