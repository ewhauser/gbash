package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestConnectionCapClosesExtraConnections(t *testing.T) {
	t.Parallel()
	srv := newTestServerState(t, time.Hour)
	srv.maxConns = 1

	first, firstServer := net.Pipe()
	defer func() { _ = first.Close() }()
	srv.addConn(firstServer)

	second, secondServer := net.Pipe()
	defer func() { _ = second.Close() }()
	srv.addConn(secondServer)

	_ = second.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := second.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("Read on connection past the cap error = %v, want EOF", err)
	}

	_ = first.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(first, `{"jsonrpc":"2.0","id":1,"method":"system.ping"}`+"\n"); err != nil {
		t.Fatalf("Write(first) error = %v", err)
	}
	line, err := bufio.NewReader(first).ReadString('\n')
	if err != nil || !strings.Contains(line, `"pong":true`) {
		t.Fatalf("first connection ping = %q, %v; want pong", line, err)
	}
}

func TestIncompleteRequestTimesOutButIdleConnectionDoesNot(t *testing.T) {
	t.Parallel()
	srv := newTestServerState(t, time.Hour)
	srv.frameTimeout = 100 * time.Millisecond

	idle, idleServer := net.Pipe()
	defer func() { _ = idle.Close() }()
	srv.addConn(idleServer)

	partial, partialServer := net.Pipe()
	defer func() { _ = partial.Close() }()
	srv.addConn(partialServer)

	_ = partial.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(partial, `{"jsonrpc":"2.0","id":1,`); err != nil {
		t.Fatalf("Write(partial) error = %v", err)
	}
	if _, err := partial.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("Read on stalled request error = %v, want EOF after the frame timeout", err)
	}

	// The idle connection has outlived the frame timeout and still works.
	time.Sleep(3 * srv.frameTimeout)
	_ = idle.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(idle, `{"jsonrpc":"2.0","id":1,"method":"system.ping"}`+"\n"); err != nil {
		t.Fatalf("Write(idle) error = %v", err)
	}
	line, err := bufio.NewReader(idle).ReadString('\n')
	if err != nil || !strings.Contains(line, `"pong":true`) {
		t.Fatalf("idle connection ping = %q, %v; want pong", line, err)
	}
}

func TestLimitFinalEnv(t *testing.T) {
	t.Parallel()
	env := map[string]string{"A": "1", "B": strings.Repeat("x", 100), "C": "3"}
	got, truncated := limitFinalEnv(env, 10)
	if !truncated || len(got) != 1 || got["A"] != "1" {
		t.Fatalf("limitFinalEnv() = %v, %t; want only A and truncated", got, truncated)
	}
	got, truncated = limitFinalEnv(env, 1000)
	if truncated || len(got) != 3 {
		t.Fatalf("limitFinalEnv() = %v, %t; want all entries", got, truncated)
	}
}
