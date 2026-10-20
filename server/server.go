package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ewhauser/gbash"
)

const (
	jsonRPCVersion = "2.0"
	defaultTTL     = 30 * time.Minute
	minReaperTick  = 100 * time.Millisecond
	maxReaperTick  = time.Second

	minAcceptRetryDelay = 5 * time.Millisecond
	maxAcceptRetryDelay = time.Second

	// DefaultMaxSessions is the session cap used when Config.MaxSessions is zero.
	DefaultMaxSessions = 256
	// DefaultMaxRequestBytes is the per-request size cap used when
	// Config.MaxRequestBytes is zero.
	DefaultMaxRequestBytes = 16 << 20
	// MaxConnections caps how many client connections are served at once.
	// Connections past the cap are closed as soon as they are accepted.
	MaxConnections = 64
	// defaultFrameTimeout bounds how long a client may take to finish sending
	// a request once it has started one.
	defaultFrameTimeout = 30 * time.Second

	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603

	rpcSessionNotFound = -32010
	rpcSessionBusy     = -32011
	rpcSessionLimit    = -32012
)

var errRequestTooLarge = errors.New("request too large")

// Config configures the shared gbash server mode.
type Config struct {
	Runtime    *gbash.Runtime
	Name       string
	Version    string
	SessionTTL time.Duration

	// MaxSessions caps how many sessions may exist at once. Zero uses
	// DefaultMaxSessions and a negative value disables the cap.
	MaxSessions int
	// MaxRequestBytes caps the encoded size of one JSON-RPC request. Zero
	// uses DefaultMaxRequestBytes and a negative value disables the cap.
	MaxRequestBytes int64
	// MaxExecTimeout, when positive, caps every session.exec call: requests
	// without a timeout_ms, or with a longer one, run with this timeout.
	MaxExecTimeout time.Duration
}

// ListenAndServeUnix serves the gbash JSON-RPC protocol on a Unix domain socket.
func ListenAndServeUnix(ctx context.Context, socketPath string, cfg Config) error {
	ln, err := listenUnixSocket(ctx, socketPath)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()
	return Serve(ctx, ln, cfg)
}

// ownedUnixListener removes its socket path on Close, but only while the path
// still names the socket this listener bound, so it never deletes a socket
// another server has since bound at the same path.
type ownedUnixListener struct {
	net.Listener
	path string
	info os.FileInfo
	once sync.Once
}

func (l *ownedUnixListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() {
		if current, statErr := os.Lstat(l.path); statErr == nil && os.SameFile(current, l.info) {
			_ = os.Remove(l.path)
		}
	})
	return err
}

func listenUnixSocket(ctx context.Context, socketPath string) (net.Listener, error) {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		return nil, fmt.Errorf("server: socket path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return nil, fmt.Errorf("server: create socket directory: %w", err)
	}
	if err := removeStaleUnixSocket(ctx, socketPath); err != nil {
		return nil, err
	}
	ln, info, err := listenPrivateUnixSocket(ctx, socketPath)
	if err != nil {
		return nil, err
	}
	return &ownedUnixListener{Listener: ln, path: socketPath, info: info}, nil
}

func unwrapOpError(err error) error {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Err != nil {
		return opErr.Err
	}
	return err
}

func unwrapPathError(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

// Serve serves the gbash JSON-RPC protocol on an existing listener.
func Serve(ctx context.Context, ln net.Listener, cfg Config) error {
	if ln == nil {
		return fmt.Errorf("server: listener is nil")
	}
	cfg = normalizeConfig(cfg)
	if cfg.Runtime == nil {
		return fmt.Errorf("server: runtime is nil")
	}

	// Canceling the server context on any exit also cancels every
	// connection context and the executions running under them.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	srv := &serverState{
		ctx:          ctx,
		cfg:          cfg,
		maxConns:     MaxConnections,
		frameTimeout: defaultFrameTimeout,
		transport:    listenerTransport(ln),
		sessions:     make(map[string]*serverSession),
		conns:        make(map[string]*clientConn),
	}

	done := make(chan struct{})
	defer func() {
		close(done)
		srv.closeConnections()
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = ln.Close()
			srv.closeConnections()
		case <-done:
		}
	}()
	go srv.reapLoop(done)

	var retryDelay time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			if isTemporaryAcceptError(err) {
				retryDelay = nextAcceptRetryDelay(retryDelay)
				srv.logf("server: accept error: %v; retrying in %v", err, retryDelay)
				timer := time.NewTimer(retryDelay)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return nil
				}
				continue
			}
			return fmt.Errorf("server: accept: %w", err)
		}
		retryDelay = 0
		srv.addConn(conn)
	}
}

// isTemporaryAcceptError reports whether an Accept failure is transient, such
// as running out of file descriptors, so the server should retry instead of
// shutting down every session.
func isTemporaryAcceptError(err error) bool {
	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) && temporary.Temporary() {
		return true
	}
	for _, errno := range []syscall.Errno{syscall.EMFILE, syscall.ENFILE, syscall.ENOBUFS, syscall.ENOMEM, syscall.ECONNABORTED, syscall.EINTR} {
		if errors.Is(err, errno) {
			return true
		}
	}
	return false
}

func nextAcceptRetryDelay(prev time.Duration) time.Duration {
	if prev <= 0 {
		return minAcceptRetryDelay
	}
	return min(prev*2, maxAcceptRetryDelay)
}

type serverState struct {
	ctx       context.Context //nolint:containedctx // intentional: server-scoped lifetime context
	cfg       Config
	transport string
	errorLog  *log.Logger // nil uses log.Default(); overridden in tests

	// maxConns and frameTimeout are fixed in Serve; tests may lower them.
	maxConns     int
	frameTimeout time.Duration

	nextID atomic.Uint64

	connsMu sync.RWMutex
	conns   map[string]*clientConn

	sessionsMu sync.Mutex
	sessions   map[string]*serverSession
}

type clientConn struct {
	server *serverState
	id     string
	conn   net.Conn
	enc    *json.Encoder
	dec    *json.Decoder
	limit  *requestLimitReader

	// ctx is canceled when the connection closes or the client goes away,
	// which cancels any session.exec running for this connection.
	ctx    context.Context //nolint:containedctx // intentional: connection-scoped lifetime context
	cancel context.CancelFunc

	writeMu sync.Mutex
	once    sync.Once
}

// decodedRequest is one framed message read off a connection, or the error
// that ended reading.
type decodedRequest struct {
	raw json.RawMessage
	err error
}

// requestLimitReader bounds how many bytes the JSON decoder may pull from the
// connection for one message. The decoder reads ahead, so one message is
// bounded by its buffered remainder plus max.
//
// It also arms a read deadline while a message is incomplete, so a client
// cannot hold a large partial request in memory indefinitely; an idle
// connection between messages has no deadline.
type requestLimitReader struct {
	r         io.Reader
	conn      net.Conn
	timeout   time.Duration
	max       int64
	remaining int64
	inFrame   bool
}

// reset starts a new message. partial reports whether bytes of the next
// message are already buffered by the decoder.
func (l *requestLimitReader) reset(partial bool) {
	l.remaining = l.max
	l.inFrame = false
	if l.conn != nil && l.timeout > 0 {
		_ = l.conn.SetReadDeadline(time.Time{})
	}
	if partial {
		l.startFrame()
	}
}

func (l *requestLimitReader) startFrame() {
	if l.inFrame {
		return
	}
	l.inFrame = true
	if l.conn != nil && l.timeout > 0 {
		_ = l.conn.SetReadDeadline(time.Now().Add(l.timeout))
	}
}

func (l *requestLimitReader) Read(p []byte) (int, error) {
	if l.max > 0 {
		if l.remaining <= 0 {
			return 0, errRequestTooLarge
		}
		if int64(len(p)) > l.remaining {
			p = p[:l.remaining]
		}
	}
	n, err := l.r.Read(p)
	if l.max > 0 {
		l.remaining -= int64(n)
	}
	if n > 0 && len(bytes.TrimSpace(p[:n])) > 0 {
		l.startFrame()
	}
	return n, err
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErrorObject `json:"error,omitempty"`
}

type rpcErrorObject struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    *rpcErrorData `json:"data,omitempty"`
}

type rpcErrorData struct {
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

type helloParams struct {
	ClientName    string `json:"client_name"`
	ClientVersion string `json:"client_version"`
}

func normalizeConfig(cfg Config) Config {
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultTTL
	}
	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.Name == "" {
		cfg.Name = "gbash"
	}
	cfg.Version = strings.TrimSpace(cfg.Version)
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.MaxSessions == 0 {
		cfg.MaxSessions = DefaultMaxSessions
	}
	if cfg.MaxRequestBytes == 0 {
		cfg.MaxRequestBytes = DefaultMaxRequestBytes
	}
	return cfg
}

// logf records details the server does not send to clients, such as
// internal errors, through the standard library's default logger.
func (s *serverState) logf(format string, args ...any) {
	logger := s.errorLog
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf(format, args...)
}

func listenerTransport(ln net.Listener) string {
	if ln == nil || ln.Addr() == nil {
		return ""
	}
	switch strings.TrimSpace(ln.Addr().Network()) {
	case "tcp4", "tcp6":
		return "tcp"
	default:
		return strings.TrimSpace(ln.Addr().Network())
	}
}

func removeStaleUnixSocket(ctx context.Context, socketPath string) error {
	info, err := os.Lstat(socketPath)
	if err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("server: path exists and is not a socket: %s", socketPath)
		}
		probeCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		conn, dialErr := (&net.Dialer{}).DialContext(probeCtx, "unix", socketPath)
		if dialErr == nil {
			_ = conn.Close()
			return fmt.Errorf("server: socket already in use: %s", socketPath)
		}
		switch {
		case errors.Is(dialErr, syscall.ECONNREFUSED), errors.Is(dialErr, syscall.ENOENT), errors.Is(dialErr, os.ErrNotExist):
		default:
			return fmt.Errorf("server: probe existing socket: %w", dialErr)
		}
		if removeErr := os.Remove(socketPath); removeErr != nil {
			return fmt.Errorf("server: remove stale socket: %w", removeErr)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("server: stat socket path: %w", err)
}

func (s *serverState) reapLoop(done <-chan struct{}) {
	tick := s.cfg.SessionTTL / 2
	switch {
	case tick <= 0:
		tick = maxReaperTick
	case tick < minReaperTick:
		tick = minReaperTick
	case tick > maxReaperTick:
		tick = maxReaperTick
	}

	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.reapExpiredSessions(time.Now().UTC())
		case <-done:
			return
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *serverState) addConn(nc net.Conn) {
	s.connsMu.Lock()
	full := s.maxConns > 0 && len(s.conns) >= s.maxConns
	s.connsMu.Unlock()
	if full {
		s.logf("server: connection limit of %d reached; closing new connection", s.maxConns)
		_ = nc.Close()
		return
	}

	limit := &requestLimitReader{r: nc, conn: nc, timeout: s.frameTimeout, max: s.cfg.MaxRequestBytes}
	ctx, cancel := context.WithCancel(s.ctx)
	conn := &clientConn{
		server: s,
		id:     s.nextIDValue("conn"),
		conn:   nc,
		enc:    json.NewEncoder(nc),
		dec:    json.NewDecoder(limit),
		limit:  limit,
		ctx:    ctx,
		cancel: cancel,
	}

	s.connsMu.Lock()
	s.conns[conn.id] = conn
	s.connsMu.Unlock()

	go conn.readLoop()
}

func (s *serverState) removeConn(id string) {
	s.connsMu.Lock()
	delete(s.conns, id)
	s.connsMu.Unlock()
}

func (s *serverState) closeConnections() {
	s.connsMu.RLock()
	conns := make([]*clientConn, 0, len(s.conns))
	for _, conn := range s.conns {
		conns = append(conns, conn)
	}
	s.connsMu.RUnlock()

	for _, conn := range conns {
		conn.close()
	}
}

func (s *serverState) nextIDValue(prefix string) string {
	return fmt.Sprintf("%s-%06d", prefix, s.nextID.Add(1))
}

// readLoop handles one connection's requests in order. A separate goroutine
// decodes them so that a client disconnect is noticed, and cancels the
// connection context, even while a session.exec is still running.
func (c *clientConn) readLoop() {
	defer c.close()

	requests := make(chan decodedRequest)
	go c.decodeLoop(requests)

	for {
		var item decodedRequest
		select {
		case <-c.ctx.Done():
			return
		case item = <-requests:
		}

		if item.err != nil {
			if errors.Is(item.err, errRequestTooLarge) {
				_ = c.writeResponse(newRPCErrorResponse(nil, rpcInvalidRequest, "request too large", &rpcErrorData{
					Code:    "REQUEST_TOO_LARGE",
					Details: fmt.Sprintf("requests are limited to %d bytes", c.server.cfg.MaxRequestBytes),
				}))
				return
			}
			_ = c.writeResponse(newRPCErrorResponse(nil, rpcParseError, "parse error", nil))
			return
		}

		req, notification, errResp := parseRPCRequest(item.raw)
		if errResp != nil {
			if err := c.writeResponse(errResp); err != nil {
				return
			}
			continue
		}

		resp := c.handleRequest(req)
		if resp == nil || notification {
			// JSON-RPC 2.0: the server must not reply to a notification.
			continue
		}
		if err := c.writeResponse(resp); err != nil {
			return
		}
	}
}

// decodeLoop reads framed messages off the connection. When the client goes
// away it cancels the connection context; malformed or oversized input is
// handed to readLoop so its error reply follows any in-flight response.
func (c *clientConn) decodeLoop(out chan<- decodedRequest) {
	for {
		c.limit.reset(hasPendingInput(c.dec))
		var raw json.RawMessage
		err := c.dec.Decode(&raw)
		if err == nil {
			c.releaseDecoderBuffer()
		}
		if err != nil && !isClientInputError(err) {
			c.cancel()
			return
		}
		select {
		case out <- decodedRequest{raw: raw, err: err}:
		case <-c.ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

// hasPendingInput reports whether the decoder holds bytes of a message it
// has not finished reading.
func hasPendingInput(dec *json.Decoder) bool {
	buffered, _ := io.ReadAll(dec.Buffered())
	return len(bytes.TrimSpace(buffered)) > 0
}

// releaseDecoderBuffer replaces the decoder so the buffer it grew for a large
// message is freed, carrying over any bytes it already read ahead.
func (c *clientConn) releaseDecoderBuffer() {
	pending, _ := io.ReadAll(c.dec.Buffered())
	var src io.Reader = c.limit
	if len(pending) > 0 {
		src = io.MultiReader(bytes.NewReader(pending), c.limit)
	}
	c.dec = json.NewDecoder(src)
}

func isClientInputError(err error) bool {
	if errors.Is(err, errRequestTooLarge) {
		return true
	}
	var syntaxErr *json.SyntaxError
	return errors.As(err, &syntaxErr)
}

// parseRPCRequest validates one request object. notification is true for a
// well-formed request without an id member, which must not get a response.
func parseRPCRequest(raw json.RawMessage) (req *rpcRequest, notification bool, errResp *rpcResponse) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false, newRPCErrorResponse(nil, rpcInvalidRequest, "invalid request", nil)
	}
	if trimmed[0] == '[' {
		return nil, false, newRPCErrorResponse(nil, rpcInvalidRequest, "batch requests are not supported", nil)
	}

	var parsed rpcRequest
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return nil, false, newRPCErrorResponse(nil, rpcInvalidRequest, "invalid request", &rpcErrorData{
			Code:    "INVALID_REQUEST",
			Details: err.Error(),
		})
	}
	if parsed.JSONRPC != jsonRPCVersion {
		return nil, false, newRPCErrorResponse(validResponseID(parsed.ID), rpcInvalidRequest, "invalid request", &rpcErrorData{
			Code:    "INVALID_REQUEST",
			Details: "jsonrpc must be \"2.0\"",
		})
	}
	notification = len(parsed.ID) == 0
	if !notification && !validRequestID(parsed.ID) {
		return nil, false, newRPCErrorResponse(nil, rpcInvalidRequest, "invalid request", &rpcErrorData{
			Code:    "INVALID_REQUEST",
			Details: "id must be a string, number, or null",
		})
	}
	if strings.TrimSpace(parsed.Method) == "" {
		return nil, false, newRPCErrorResponse(validResponseID(parsed.ID), rpcInvalidRequest, "invalid request", &rpcErrorData{
			Code:    "INVALID_REQUEST",
			Details: "method must not be empty",
		})
	}
	return &parsed, notification, nil
}

func validRequestID(id json.RawMessage) bool {
	trimmed := bytes.TrimSpace(id)
	if len(trimmed) == 0 {
		return false
	}
	switch trimmed[0] {
	case '"', 'n', '-':
		return true
	default:
		return trimmed[0] >= '0' && trimmed[0] <= '9'
	}
}

func validResponseID(id json.RawMessage) json.RawMessage {
	if validRequestID(id) {
		return id
	}
	return nil
}

func (c *clientConn) handleRequest(req *rpcRequest) *rpcResponse {
	if req == nil {
		return newRPCErrorResponse(nil, rpcInvalidRequest, "invalid request", nil)
	}

	switch req.Method {
	case "system.hello", "hello":
		if _, err := decodeParams[helloParams](req.Params); err != nil {
			return invalidParamsResponse(req.ID, err)
		}
		return newRPCResultResponse(req.ID, helloResult{
			ServerName:    c.server.cfg.Name,
			ServerVersion: c.server.cfg.Version,
			Protocol:      jsonRPCVersion,
			Capabilities: helloCapabilities{
				Binary:             c.server.cfg.Name,
				Transport:          c.server.transport,
				PersistentSessions: true,
				SessionExec:        true,
				FileSystemRPC:      false,
				InteractiveShell:   false,
			},
		})
	case "system.ping", "ping":
		if _, err := decodeParams[map[string]any](req.Params); err != nil {
			return invalidParamsResponse(req.ID, err)
		}
		return newRPCResultResponse(req.ID, map[string]any{"pong": true})
	case "session.create":
		if _, err := decodeParams[map[string]any](req.Params); err != nil {
			return invalidParamsResponse(req.ID, err)
		}
		session, err := c.server.createSession(c.id)
		if err != nil {
			return c.server.appErrorResponse(req.ID, err)
		}
		return newRPCResultResponse(req.ID, sessionCreateResult{Session: session.summary()})
	case "session.get":
		params, err := decodeParams[sessionIDParams](req.Params)
		if err != nil {
			return invalidParamsResponse(req.ID, err)
		}
		session, err := c.server.lookupSession(c.id, params.SessionID)
		if err != nil {
			return c.server.appErrorResponse(req.ID, err)
		}
		return newRPCResultResponse(req.ID, sessionGetResult{Session: session.summary()})
	case "session.list":
		if _, err := decodeParams[map[string]any](req.Params); err != nil {
			return invalidParamsResponse(req.ID, err)
		}
		c.server.reapExpiredSessions(time.Now().UTC())
		return newRPCResultResponse(req.ID, sessionListResult{Sessions: c.server.listSessions(c.id)})
	case "session.destroy":
		params, err := decodeParams[sessionIDParams](req.Params)
		if err != nil {
			return invalidParamsResponse(req.ID, err)
		}
		session, err := c.server.destroySession(c.id, params.SessionID)
		if err != nil {
			return c.server.appErrorResponse(req.ID, err)
		}
		return newRPCResultResponse(req.ID, sessionGetResult{Session: session.summary()})
	case "session.exec":
		params, err := decodeParams[sessionExecParams](req.Params)
		if err != nil {
			return invalidParamsResponse(req.ID, err)
		}
		if err := params.validate(); err != nil {
			return invalidParamsResponse(req.ID, err)
		}
		session, err := c.server.acquireSession(c.id, params.SessionID)
		if err != nil {
			return c.server.appErrorResponse(req.ID, err)
		}
		result, err := session.exec(c.ctx, &params)
		if err != nil {
			return c.server.appErrorResponse(req.ID, err)
		}
		return newRPCResultResponse(req.ID, result)
	default:
		return newRPCErrorResponse(req.ID, rpcMethodNotFound, "method not found", nil)
	}
}

func (c *clientConn) writeResponse(resp *rpcResponse) error {
	if resp == nil {
		return nil
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.enc.Encode(resp)
}

func (c *clientConn) close() {
	c.once.Do(func() {
		c.cancel()
		_ = c.conn.Close()
		c.server.removeConn(c.id)
		c.server.dropOwnedSessions(c.id)
	})
}

func newRPCResultResponse(id json.RawMessage, result any) *rpcResponse {
	return &rpcResponse{
		JSONRPC: jsonRPCVersion,
		ID:      responseID(id),
		Result:  result,
	}
}

func newRPCErrorResponse(id json.RawMessage, code int, message string, data *rpcErrorData) *rpcResponse {
	return &rpcResponse{
		JSONRPC: jsonRPCVersion,
		ID:      responseID(id),
		Error: &rpcErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

func responseID(id json.RawMessage) json.RawMessage {
	if validRequestID(id) {
		return id
	}
	return json.RawMessage("null")
}

func invalidParamsResponse(id json.RawMessage, err error) *rpcResponse {
	return newRPCErrorResponse(id, rpcInvalidParams, "invalid params", &rpcErrorData{
		Code:    "INVALID_ARGUMENT",
		Details: err.Error(),
	})
}

// internalErrorResponse logs err and returns a generic error, since internal
// failures can carry host details such as absolute paths.
func (s *serverState) internalErrorResponse(id json.RawMessage, err error) *rpcResponse {
	s.logf("server: internal error: %v", err)
	return newRPCErrorResponse(id, rpcInternalError, "internal error", &rpcErrorData{
		Code: "INTERNAL",
	})
}

func (s *serverState) appErrorResponse(id json.RawMessage, err error) *rpcResponse {
	switch {
	case errors.Is(err, errInvalidArgument):
		return invalidParamsResponse(id, err)
	case errors.Is(err, errSessionNotFound):
		return newRPCErrorResponse(id, rpcSessionNotFound, "session not found", &rpcErrorData{
			Code:    "SESSION_NOT_FOUND",
			Details: err.Error(),
		})
	case errors.Is(err, errSessionBusy):
		return newRPCErrorResponse(id, rpcSessionBusy, "session is busy", &rpcErrorData{
			Code:    "SESSION_BUSY",
			Details: err.Error(),
		})
	case errors.Is(err, errSessionLimit):
		return newRPCErrorResponse(id, rpcSessionLimit, "session limit reached", &rpcErrorData{
			Code:    "SESSION_LIMIT",
			Details: err.Error(),
		})
	default:
		return s.internalErrorResponse(id, err)
	}
}

func decodeParams[T any](raw json.RawMessage) (T, error) {
	var out T
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return out, nil
	}
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return out, fmt.Errorf("%w: %w", errInvalidArgument, err)
	}
	return out, nil
}
