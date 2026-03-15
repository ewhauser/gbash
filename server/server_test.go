package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ewhauser/gbash"
	gbserver "github.com/ewhauser/gbash/server"
)

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    *rpcErrorData `json:"data"`
}

type rpcErrorData struct {
	Code    string `json:"code"`
	Details string `json:"details"`
}

type helloResult struct {
	ServerName    string `json:"server_name"`
	ServerVersion string `json:"server_version"`
	Protocol      string `json:"protocol"`
	Capabilities  struct {
		Binary             string `json:"binary"`
		Transport          string `json:"transport"`
		PersistentSessions bool   `json:"persistent_sessions"`
		SessionExec        bool   `json:"session_exec"`
		FileSystemRPC      bool   `json:"filesystem_rpc"`
		InteractiveShell   bool   `json:"interactive_shell"`
	} `json:"capabilities"`
}

type sessionResult struct {
	Session struct {
		SessionID string `json:"session_id"`
		State     string `json:"state"`
	} `json:"session"`
}

type sessionListResult struct {
	Sessions []struct {
		SessionID string `json:"session_id"`
		State     string `json:"state"`
	} `json:"sessions"`
}

type execResult struct {
	SessionID       string            `json:"session_id"`
	ExitCode        int               `json:"exit_code"`
	Stdout          string            `json:"stdout"`
	Stderr          string            `json:"stderr"`
	StdoutTruncated bool              `json:"stdout_truncated"`
	StderrTruncated bool              `json:"stderr_truncated"`
	FinalEnv        map[string]string `json:"final_env"`
	ShellExited     bool              `json:"shell_exited"`
	ControlStderr   string            `json:"control_stderr"`
	Session         struct {
		SessionID string `json:"session_id"`
		State     string `json:"state"`
	} `json:"session"`
}

type serverHandle struct {
	socket string
	cancel context.CancelFunc
	errCh  chan error
}

type testClient struct {
	t      *testing.T
	conn   net.Conn
	enc    *json.Encoder
	dec    *json.Decoder
	nextID atomic.Uint64
}

func TestServerSessionLifecycleAndExec(t *testing.T) {
	srv := startServer(t, gbserver.Config{
		Name:       "gbash",
		Version:    "test",
		SessionTTL: time.Second,
	})
	client := dialClient(t, srv.socket)
	defer client.Close()

	hello := client.call("system.hello", map[string]any{"client_name": "test"})
	mustOK(t, &hello)
	var helloPayload helloResult
	decodeResult(t, &hello, &helloPayload)
	if helloPayload.Protocol != "2.0" {
		t.Fatalf("protocol = %q, want 2.0", helloPayload.Protocol)
	}
	if helloPayload.Capabilities.Transport != "unix" || !helloPayload.Capabilities.PersistentSessions || !helloPayload.Capabilities.SessionExec {
		t.Fatalf("capabilities = %+v, want unix persistent session exec", helloPayload.Capabilities)
	}
	if helloPayload.Capabilities.FileSystemRPC || helloPayload.Capabilities.InteractiveShell {
		t.Fatalf("capabilities = %+v, want no fs rpc or interactive shell", helloPayload.Capabilities)
	}

	create := client.call("session.create", nil)
	mustOK(t, &create)
	var created sessionResult
	decodeResult(t, &create, &created)
	sessionID := created.Session.SessionID
	if sessionID == "" || created.Session.State != "idle" {
		t.Fatalf("session.create result = %+v, want idle session id", created)
	}

	list := client.call("session.list", nil)
	mustOK(t, &list)
	var listed sessionListResult
	decodeResult(t, &list, &listed)
	if len(listed.Sessions) != 1 || listed.Sessions[0].SessionID != sessionID {
		t.Fatalf("session.list result = %+v, want only %s", listed, sessionID)
	}

	exec := client.call("session.exec", map[string]any{
		"session_id": sessionID,
		"script":     "printf 'hello\\n'; printf 'warn\\n' >&2; export MODE=debug\n",
	})
	mustOK(t, &exec)
	var execPayload execResult
	decodeResult(t, &exec, &execPayload)
	if execPayload.ExitCode != 0 || execPayload.Stdout != "hello\n" || execPayload.Stderr != "warn\n" {
		t.Fatalf("session.exec result = %+v, want hello/warn exit 0", execPayload)
	}
	if execPayload.FinalEnv["MODE"] != "debug" {
		t.Fatalf("final env = %+v, want MODE=debug", execPayload.FinalEnv)
	}
	if execPayload.Session.State != "idle" {
		t.Fatalf("session.exec session state = %+v, want idle after completion", execPayload.Session)
	}

	get := client.call("session.get", map[string]any{"session_id": sessionID})
	mustOK(t, &get)
	var got sessionResult
	decodeResult(t, &get, &got)
	if got.Session.State != "idle" {
		t.Fatalf("session.get result = %+v, want idle", got)
	}

	destroy := client.call("session.destroy", map[string]any{"session_id": sessionID})
	mustOK(t, &destroy)
	decodeResult(t, &destroy, &got)
	if got.Session.SessionID != sessionID {
		t.Fatalf("session.destroy result = %+v, want destroyed session %s", got, sessionID)
	}

	list = client.call("session.list", nil)
	mustOK(t, &list)
	decodeResult(t, &list, &listed)
	if len(listed.Sessions) != 0 {
		t.Fatalf("session.list result = %+v, want empty after destroy", listed)
	}
}

func TestServerConcurrentSessionsAndBusy(t *testing.T) {
	srv := startServer(t, gbserver.Config{
		Name:       "gbash",
		Version:    "test",
		SessionTTL: time.Second,
	})
	client1 := dialClient(t, srv.socket)
	defer client1.Close()
	client2 := dialClient(t, srv.socket)
	defer client2.Close()

	session1 := mustCreateSession(t, client1)
	session2 := mustCreateSession(t, client1)

	done := make(chan rpcResponse, 1)
	go func() {
		done <- client1.call("session.exec", map[string]any{
			"session_id": session1,
			"script":     "sleep 0.2\nprintf 'one\\n'",
		})
	}()

	waitForSessionState(t, client2, session1, "running")

	busy := client2.call("session.exec", map[string]any{
		"session_id": session1,
		"script":     "printf 'no\\n'",
	})
	if busy.Error == nil || busy.Error.Data == nil || busy.Error.Data.Code != "SESSION_BUSY" {
		t.Fatalf("busy response = %#v, want SESSION_BUSY", busy)
	}

	ok := client2.call("session.exec", map[string]any{
		"session_id": session2,
		"script":     "printf 'two\\n'",
	})
	mustOK(t, &ok)
	var okPayload execResult
	decodeResult(t, &ok, &okPayload)
	if okPayload.Stdout != "two\n" || okPayload.ExitCode != 0 {
		t.Fatalf("second session exec = %+v, want exit 0 and two", okPayload)
	}

	first := <-done
	mustOK(t, &first)
	var firstPayload execResult
	decodeResult(t, &first, &firstPayload)
	if firstPayload.Stdout != "one\n" || firstPayload.ExitCode != 0 {
		t.Fatalf("first session exec = %+v, want exit 0 and one", firstPayload)
	}
}

func TestServerSessionTTLExpiry(t *testing.T) {
	srv := startServer(t, gbserver.Config{
		Name:       "gbash",
		Version:    "test",
		SessionTTL: 250 * time.Millisecond,
	})
	client := dialClient(t, srv.socket)
	defer client.Close()

	sessionID := mustCreateSession(t, client)

	time.Sleep(500 * time.Millisecond)

	list := client.call("session.list", nil)
	mustOK(t, &list)
	var listed sessionListResult
	decodeResult(t, &list, &listed)
	if len(listed.Sessions) != 0 {
		t.Fatalf("session.list result = %+v, want empty after ttl expiry", listed)
	}

	get := client.call("session.get", map[string]any{"session_id": sessionID})
	if get.Error == nil || get.Error.Data == nil || get.Error.Data.Code != "SESSION_NOT_FOUND" {
		t.Fatalf("session.get result = %#v, want SESSION_NOT_FOUND", get)
	}
}

func TestServeTCPListenerReportsTCPTransport(t *testing.T) {
	rt, err := gbash.New()
	if err != nil {
		t.Fatalf("gbash.New() error = %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen(tcp) error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- gbserver.Serve(ctx, ln, gbserver.Config{
			Runtime:    rt,
			Name:       "gbash",
			Version:    "test",
			SessionTTL: time.Second,
		})
	}()

	client := &testClient{
		t:    t,
		conn: mustDialTCP(t, ln.Addr().String()),
	}
	client.enc = json.NewEncoder(client.conn)
	client.dec = json.NewDecoder(client.conn)
	defer client.Close()

	resp := client.call("system.hello", map[string]any{"client_name": "test"})
	mustOK(t, &resp)

	var payload helloResult
	decodeResult(t, &resp, &payload)
	if got, want := payload.Capabilities.Transport, "tcp"; got != want {
		t.Fatalf("transport = %q, want %q", got, want)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve(tcp) error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tcp server shutdown")
	}
}

func startServer(t *testing.T, cfg gbserver.Config, opts ...gbash.Option) *serverHandle {
	t.Helper()

	rt, err := gbash.New(opts...)
	if err != nil {
		t.Fatalf("gbash.New() error = %v", err)
	}
	cfg.Runtime = rt

	ctx, cancel := context.WithCancel(context.Background())
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("gbs-%d.sock", time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.Remove(socket) })
	errCh := make(chan error, 1)
	go func() {
		errCh <- gbserver.ListenAndServeUnix(ctx, socket, cfg)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", socket, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	handle := &serverHandle{
		socket: socket,
		cancel: cancel,
		errCh:  errCh,
	}
	t.Cleanup(func() {
		handle.cancel()
		select {
		case err := <-handle.errCh:
			if err != nil {
				t.Fatalf("server exited with error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for server shutdown")
		}
	})
	return handle
}

func dialClient(t *testing.T, socket string) *testClient {
	t.Helper()

	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		t.Fatalf("DialTimeout(%q) error = %v", socket, err)
	}
	return &testClient{
		t:    t,
		conn: conn,
		enc:  json.NewEncoder(conn),
		dec:  json.NewDecoder(conn),
	}
}

func mustDialTCP(t *testing.T, addr string) net.Conn {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("DialTimeout(%q) error = %v", addr, err)
	}
	return conn
}

func (c *testClient) Close() {
	if c == nil || c.conn == nil {
		return
	}
	_ = c.conn.Close()
}

func (c *testClient) call(method string, params any) rpcResponse {
	c.t.Helper()

	id := fmt.Sprintf("req-%d", c.nextID.Add(1))
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		request["params"] = params
	}
	if err := c.enc.Encode(request); err != nil {
		c.t.Fatalf("Encode(%s) error = %v", method, err)
	}
	if err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		c.t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var resp rpcResponse
	if err := c.dec.Decode(&resp); err != nil {
		c.t.Fatalf("Decode(%s) error = %v", method, err)
	}
	return resp
}

func waitForSessionState(t *testing.T, client *testClient, sessionID, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp := client.call("session.get", map[string]any{"session_id": sessionID})
		if resp.Error == nil {
			var payload sessionResult
			decodeResult(t, &resp, &payload)
			if payload.Session.State == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session %s to reach state %q", sessionID, want)
}

func mustCreateSession(t *testing.T, client *testClient) string {
	t.Helper()

	resp := client.call("session.create", nil)
	mustOK(t, &resp)

	var payload sessionResult
	decodeResult(t, &resp, &payload)
	if payload.Session.SessionID == "" {
		t.Fatalf("session.create result = %+v, want session id", payload)
	}
	return payload.Session.SessionID
}

func mustOK(t *testing.T, resp *rpcResponse) {
	t.Helper()
	if resp == nil || resp.Error != nil {
		t.Fatalf("response = %#v, want success", resp)
	}
	if resp.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
}

func decodeResult[T any](t *testing.T, resp *rpcResponse, out *T) {
	t.Helper()
	if resp == nil || len(resp.Result) == 0 {
		t.Fatalf("response = %#v, want result payload", resp)
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		t.Fatalf("Unmarshal(result) error = %v; raw=%s", err, string(resp.Result))
	}
}

func TestServerRejectsInvalidMethod(t *testing.T) {
	srv := startServer(t, gbserver.Config{
		Name:       "gbash",
		Version:    "test",
		SessionTTL: time.Second,
	})
	client := dialClient(t, srv.socket)
	defer client.Close()

	resp := client.call("session.stream", nil)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("response = %#v, want method not found", resp)
	}
}

func TestServerExecCarriesSessionStateInResult(t *testing.T) {
	srv := startServer(t, gbserver.Config{
		Name:       "gbash",
		Version:    "test",
		SessionTTL: time.Second,
	})
	client := dialClient(t, srv.socket)
	defer client.Close()

	sessionID := mustCreateSession(t, client)
	resp := client.call("session.exec", map[string]any{
		"session_id": sessionID,
		"script":     "cd /tmp\npwd\n",
	})
	mustOK(t, &resp)

	var payload execResult
	decodeResult(t, &resp, &payload)
	if got, want := payload.Stdout, "/tmp\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if payload.Session.State != "idle" {
		t.Fatalf("session = %+v, want idle summary in exec result", payload.Session)
	}
	if payload.Session.SessionID != sessionID {
		t.Fatalf("session = %+v, want session %s", payload.Session, sessionID)
	}
	if !strings.Contains(payload.FinalEnv["PWD"], "/tmp") {
		t.Fatalf("final env = %+v, want PWD to end in /tmp", payload.FinalEnv)
	}
}
