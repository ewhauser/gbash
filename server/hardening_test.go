package server_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/commands"
	gbserver "github.com/ewhauser/gbash/server"
)

func TestServerClientDisconnectCancelsRunningExec(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	canceled := make(chan struct{})
	block := commands.DefineCommand("block", func(ctx context.Context, _ *commands.Invocation) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	})
	srv := startServer(t, gbserver.Config{}, gbash.WithRegistry(registryWithCommands(t, block)))

	owner := dialClient(t, srv.socket)
	sessionID := mustCreateSession(t, owner)
	if err := owner.enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      "exec-1",
		"method":  "session.exec",
		"params":  map[string]any{"session_id": sessionID, "script": "block\n"},
	}); err != nil {
		t.Fatalf("Encode(session.exec) error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for exec to start")
	}
	owner.Close()

	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("exec was not canceled after its client disconnected")
	}

	other := dialClient(t, srv.socket)
	defer other.Close()
	resp := other.call("session.get", map[string]any{"session_id": sessionID})
	if resp.Error == nil || resp.Error.Data == nil || resp.Error.Data.Code != "SESSION_NOT_FOUND" {
		t.Fatalf("session.get after owner disconnect = %+v, want SESSION_NOT_FOUND", resp.Error)
	}
}

func TestServerMaxExecTimeoutCapsRequests(t *testing.T) {
	t.Parallel()

	srv := startServer(t, gbserver.Config{MaxExecTimeout: 50 * time.Millisecond})
	client := dialClient(t, srv.socket)
	defer client.Close()
	sessionID := mustCreateSession(t, client)

	for _, timeoutMs := range []int64{0, 60_000} {
		resp := client.call("session.exec", map[string]any{
			"session_id": sessionID,
			"script":     "sleep 10\necho late\n",
			"timeout_ms": timeoutMs,
		})
		mustOK(t, &resp)
		var result execResult
		decodeResult(t, &resp, &result)
		if result.ExitCode != 124 || result.Stdout != "" {
			t.Fatalf("timeout_ms=%d: exit=%d stdout=%q, want 124 and no output", timeoutMs, result.ExitCode, result.Stdout)
		}
	}
}

func TestServerHugeTimeoutDoesNotOverflow(t *testing.T) {
	t.Parallel()

	srv := startServer(t, gbserver.Config{})
	client := dialClient(t, srv.socket)
	defer client.Close()
	sessionID := mustCreateSession(t, client)

	resp := client.call("session.exec", map[string]any{
		"session_id": sessionID,
		"script":     "sleep 0.05\necho done\n",
		"timeout_ms": int64(18446744073710),
	})
	mustOK(t, &resp)
	var result execResult
	decodeResult(t, &resp, &result)
	if result.ExitCode != 0 || result.Stdout != "done\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 0 and done", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestServerExecReportsNonUTF8OutputAsBase64(t *testing.T) {
	t.Parallel()

	srv := startServer(t, gbserver.Config{})
	client := dialClient(t, srv.socket)
	defer client.Close()
	sessionID := mustCreateSession(t, client)

	resp := client.call("session.exec", map[string]any{
		"session_id": sessionID,
		"script":     `printf '\377ok'; printf 'caf\303\251' >&2`,
	})
	mustOK(t, &resp)
	var result struct {
		Stdout       string  `json:"stdout"`
		StdoutBase64 *string `json:"stdout_base64"`
		Stderr       string  `json:"stderr"`
		StderrBase64 *string `json:"stderr_base64"`
	}
	decodeResult(t, &resp, &result)
	if result.StdoutBase64 == nil {
		t.Fatalf("stdout_base64 missing for non-UTF-8 stdout; raw=%s", resp.Result)
	}
	raw, err := base64.StdEncoding.DecodeString(*result.StdoutBase64)
	if err != nil {
		t.Fatalf("DecodeString(stdout_base64) error = %v", err)
	}
	if got, want := string(raw), "\xffok"; got != want {
		t.Fatalf("stdout bytes = %q, want %q", got, want)
	}
	if got, want := result.Stdout, "�ok"; got != want {
		t.Fatalf("stdout = %q, want lossy %q", got, want)
	}
	if result.StderrBase64 != nil {
		t.Fatalf("stderr_base64 = %q, want omitted for valid UTF-8", *result.StderrBase64)
	}
	if got, want := result.Stderr, "café"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestServerDoesNotReplyToNotifications(t *testing.T) {
	t.Parallel()

	srv := startServer(t, gbserver.Config{})
	client := dialClient(t, srv.socket)
	defer client.Close()

	for _, notification := range []map[string]any{
		{"jsonrpc": "2.0", "method": "system.ping"},
		{"jsonrpc": "2.0", "method": "no.such.method"},
		{"jsonrpc": "2.0", "method": "session.get", "params": map[string]any{"session_id": "sess-missing"}},
	} {
		if err := client.enc.Encode(notification); err != nil {
			t.Fatalf("Encode(notification) error = %v", err)
		}
	}
	resp := client.call("system.ping", nil)
	mustOK(t, &resp)
	if got, want := string(resp.ID), `"req-1"`; got != want {
		t.Fatalf("first response id = %s, want %s (notifications must not be answered)", got, want)
	}
}

func TestServerSessionIDsAreUnguessable(t *testing.T) {
	t.Parallel()

	srv := startServer(t, gbserver.Config{})
	client := dialClient(t, srv.socket)
	defer client.Close()

	pattern := regexp.MustCompile(`^sess-[0-9a-f]{32}$`)
	first := mustCreateSession(t, client)
	second := mustCreateSession(t, client)
	for _, id := range []string{first, second} {
		if !pattern.MatchString(id) {
			t.Fatalf("session id = %q, want sess- followed by 32 random hex digits", id)
		}
	}
	if first == second {
		t.Fatalf("session ids repeat: %q", first)
	}

	resp := client.call("session.list", nil)
	mustOK(t, &resp)
	var list sessionListResult
	decodeResult(t, &resp, &list)
	if len(list.Sessions) != 2 || list.Sessions[0].SessionID != first || list.Sessions[1].SessionID != second {
		t.Fatalf("session.list = %+v, want [%s %s] in creation order", list.Sessions, first, second)
	}
}

func TestServerMaxSessions(t *testing.T) {
	t.Parallel()

	srv := startServer(t, gbserver.Config{MaxSessions: 2})
	client := dialClient(t, srv.socket)
	defer client.Close()

	first := mustCreateSession(t, client)
	_ = mustCreateSession(t, client)

	resp := client.call("session.create", nil)
	if resp.Error == nil || resp.Error.Code != -32012 || resp.Error.Data == nil || resp.Error.Data.Code != "SESSION_LIMIT" {
		t.Fatalf("third session.create = %+v, want SESSION_LIMIT", resp.Error)
	}

	resp = client.call("session.destroy", map[string]any{"session_id": first})
	mustOK(t, &resp)
	_ = mustCreateSession(t, client)
}

func TestServerRejectsOversizedRequest(t *testing.T) {
	t.Parallel()

	srv := startServer(t, gbserver.Config{MaxRequestBytes: 1024})
	client := dialClient(t, srv.socket)
	defer client.Close()

	// Requests under the cap still work.
	sessionID := mustCreateSession(t, client)

	// The server stops reading and closes mid-request, so the write itself
	// may fail with a broken pipe; only the reply matters.
	go func() {
		_ = client.enc.Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      "big",
			"method":  "session.exec",
			"params":  map[string]any{"session_id": sessionID, "script": "echo " + strings.Repeat("x", 8192)},
		})
	}()
	if err := client.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var resp rpcResponse
	if err := client.dec.Decode(&resp); err != nil {
		t.Fatalf("Decode(error response) error = %v", err)
	}
	if resp.Error == nil || resp.Error.Data == nil || resp.Error.Data.Code != "REQUEST_TOO_LARGE" {
		t.Fatalf("response = %+v, want REQUEST_TOO_LARGE", resp)
	}
	// The server closes with unread request bytes queued, which Linux
	// reports to the peer as a reset rather than a clean EOF.
	var extra json.RawMessage
	if err := client.dec.Decode(&extra); !errors.Is(err, io.EOF) && !errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("Decode after oversized request error = %v, want EOF or ECONNRESET (connection closed)", err)
	}
}

// flakyListener fails its first Accept calls with EMFILE.
type flakyListener struct {
	net.Listener
	failures atomic.Int32
}

func (l *flakyListener) Accept() (net.Conn, error) {
	if l.failures.Add(-1) >= 0 {
		return nil, &net.OpError{Op: "accept", Net: "unix", Err: os.NewSyscallError("accept", syscall.EMFILE)}
	}
	return l.Listener.Accept()
}

func TestServeRetriesTemporaryAcceptErrors(t *testing.T) {
	t.Parallel()

	rt, err := gbash.New()
	if err != nil {
		t.Fatalf("gbash.New() error = %v", err)
	}
	socket := newSocketPath(t)
	inner, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("Listen(unix) error = %v", err)
	}
	ln := &flakyListener{Listener: inner}
	ln.failures.Store(3)

	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() { errCh <- gbserver.Serve(ctx, ln, gbserver.Config{Runtime: rt}) }()

	client := dialClient(t, socket)
	resp := client.call("system.ping", nil)
	mustOK(t, &resp)
	client.Close()

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() error = %v, want nil after temporary accept errors", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Serve to return")
	}
}

// failingListener accepts normally until fail is closed, then returns a
// permanent error.
type failingListener struct {
	net.Listener
	accepted chan struct{}
	fail     chan struct{}
	count    atomic.Int32
}

func (l *failingListener) Accept() (net.Conn, error) {
	if l.count.Add(1) > 1 {
		<-l.fail
		return nil, errors.New("permanent accept failure")
	}
	conn, err := l.Listener.Accept()
	close(l.accepted)
	return conn, err
}

func TestServeClosesLiveConnectionsOnAcceptFailure(t *testing.T) {
	t.Parallel()

	rt, err := gbash.New()
	if err != nil {
		t.Fatalf("gbash.New() error = %v", err)
	}
	socket := newSocketPath(t)
	inner, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("Listen(unix) error = %v", err)
	}
	defer func() { _ = inner.Close() }()
	ln := &failingListener{Listener: inner, accepted: make(chan struct{}), fail: make(chan struct{})}

	errCh := make(chan error, 1)
	go func() { errCh <- gbserver.Serve(t.Context(), ln, gbserver.Config{Runtime: rt}) }()

	client := dialClient(t, socket)
	defer client.Close()
	resp := client.call("system.ping", nil)
	mustOK(t, &resp)

	close(ln.fail)
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "permanent accept failure") {
			t.Fatalf("Serve() error = %v, want permanent accept failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Serve to return")
	}

	if err := client.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var extra json.RawMessage
	if err := client.dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("Decode after Serve exit error = %v, want EOF (connection closed)", err)
	}
}

func TestServerFinalEnvIsOptIn(t *testing.T) {
	t.Parallel()

	srv := startServer(t, gbserver.Config{})
	client := dialClient(t, srv.socket)
	defer client.Close()
	sessionID := mustCreateSession(t, client)

	resp := client.call("session.exec", map[string]any{"session_id": sessionID, "script": "export MODE=debug\n"})
	mustOK(t, &resp)
	if strings.Contains(string(resp.Result), "final_env") {
		t.Fatalf("result = %s, want no final_env unless requested", resp.Result)
	}

	resp = client.call("session.exec", map[string]any{"session_id": sessionID, "script": "export MODE=debug\n", "include_final_env": true})
	mustOK(t, &resp)
	var result execResult
	decodeResult(t, &resp, &result)
	if result.FinalEnv["MODE"] != "debug" {
		t.Fatalf("final_env = %v, want MODE=debug", result.FinalEnv)
	}
}

func TestServerExecRejectsBadOptionsAsInvalidArgument(t *testing.T) {
	t.Parallel()

	srv := startServer(t, gbserver.Config{})
	client := dialClient(t, srv.socket)
	defer client.Close()
	sessionID := mustCreateSession(t, client)

	for _, params := range []map[string]any{
		{"session_id": sessionID, "script": "echo hi\n", "shell_variant": "weird"},
		{"session_id": sessionID, "script": "echo hi\n", "startup_options": []string{"bogus"}},
	} {
		resp := client.call("session.exec", params)
		if resp.Error == nil || resp.Error.Code != -32602 || resp.Error.Data == nil || resp.Error.Data.Code != "INVALID_ARGUMENT" {
			t.Fatalf("session.exec(%v) error = %+v, want -32602 INVALID_ARGUMENT", params, resp.Error)
		}
	}
	resp := client.call("session.exec", map[string]any{"session_id": sessionID, "script": "echo ok\n", "startup_options": []string{"pipefail"}})
	mustOK(t, &resp)
}
