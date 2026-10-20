package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/internal/shell/interp"
)

// maxFinalEnvBytes caps the encoded size of final_env in one response.
const maxFinalEnvBytes = 1 << 20

var (
	errInvalidArgument = errors.New("invalid argument")
	errSessionNotFound = errors.New("session not found")
	errSessionBusy     = errors.New("session is busy")
	errSessionLimit    = errors.New("session limit reached")
)

type helloResult struct {
	ServerName    string            `json:"server_name"`
	ServerVersion string            `json:"server_version"`
	Protocol      string            `json:"protocol"`
	Capabilities  helloCapabilities `json:"capabilities"`
}

type helloCapabilities struct {
	Binary             string `json:"binary"`
	Transport          string `json:"transport"`
	PersistentSessions bool   `json:"persistent_sessions"`
	SessionExec        bool   `json:"session_exec"`
	FileSystemRPC      bool   `json:"filesystem_rpc"`
	InteractiveShell   bool   `json:"interactive_shell"`
}

type sessionSummary struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type sessionCreateResult struct {
	Session sessionSummary `json:"session"`
}

type sessionGetResult struct {
	Session sessionSummary `json:"session"`
}

type sessionListResult struct {
	Sessions []sessionSummary `json:"sessions"`
}

type sessionIDParams struct {
	SessionID string `json:"session_id"`
}

type sessionExecParams struct {
	SessionID      string             `json:"session_id"`
	Name           string             `json:"name"`
	Script         string             `json:"script"`
	ShellVariant   gbash.ShellVariant `json:"shell_variant"`
	Args           []string           `json:"args"`
	StartupOptions []string           `json:"startup_options"`
	Env            map[string]string  `json:"env"`
	WorkDir        string             `json:"work_dir"`
	ReplaceEnv     bool               `json:"replace_env"`
	TimeoutMs      int64              `json:"timeout_ms"`
	// IncludeFinalEnv asks for final_env in the result. It is off by default
	// because the shell environment can be large and holds inherited values.
	IncludeFinalEnv bool `json:"include_final_env"`
}

// validate rejects client input the runtime would otherwise report as an
// internal failure.
func (p *sessionExecParams) validate() error {
	if !p.ShellVariant.Normalize().Valid() {
		return fmt.Errorf("%w: invalid shell_variant %q", errInvalidArgument, p.ShellVariant)
	}
	for _, option := range p.StartupOptions {
		if strings.TrimSpace(option) == "" {
			continue
		}
		if !interp.IsSetOption(option) {
			return fmt.Errorf("%w: invalid startup option %q", errInvalidArgument, option)
		}
	}
	return nil
}

type sessionExecResult struct {
	SessionID string `json:"session_id"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	// StdoutBase64 and StderrBase64 carry the exact bytes, base64-encoded,
	// only when the stream is not valid UTF-8; Stdout and Stderr then hold a
	// lossy copy with invalid bytes replaced by U+FFFD.
	StdoutBase64    string            `json:"stdout_base64,omitempty"`
	StderrBase64    string            `json:"stderr_base64,omitempty"`
	StdoutTruncated bool              `json:"stdout_truncated"`
	StderrTruncated bool              `json:"stderr_truncated"`
	FinalEnv        map[string]string `json:"final_env,omitempty"`
	// FinalEnvTruncated reports that final_env was cut short to stay under
	// maxFinalEnvBytes.
	FinalEnvTruncated bool           `json:"final_env_truncated,omitempty"`
	ShellExited       bool           `json:"shell_exited"`
	ControlStderr     string         `json:"control_stderr,omitempty"`
	StartedAt         string         `json:"started_at,omitempty"`
	FinishedAt        string         `json:"finished_at,omitempty"`
	DurationMs        float64        `json:"duration_ms"`
	Session           sessionSummary `json:"session"`
}

type serverSession struct {
	server *serverState
	id     string
	// owner is the id of the connection that created the session; only that
	// connection may see or use it.
	owner string
	shell *gbash.Session

	createdAt time.Time
	updatedAt time.Time
	idleSince time.Time

	mu   sync.Mutex
	busy bool
}

func (s *serverState) createSession(owner string) (*serverSession, error) {
	s.reapExpiredSessions(time.Now().UTC())

	// Check the cap before building a session so a full server does not pay
	// for one, and again on insert since creates can race.
	if err := s.checkSessionLimit(); err != nil {
		return nil, err
	}

	id, err := newSessionID()
	if err != nil {
		return nil, fmt.Errorf("create session id: %w", err)
	}

	sessionShell, err := s.cfg.Runtime.NewSession(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("create gbash session: %w", err)
	}

	now := time.Now().UTC()
	session := &serverSession{
		server:    s,
		id:        id,
		owner:     owner,
		shell:     sessionShell,
		createdAt: now,
		updatedAt: now,
		idleSince: now,
	}

	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	if err := s.checkSessionLimitLocked(); err != nil {
		return nil, err
	}
	s.sessions[session.id] = session
	return session, nil
}

func (s *serverState) checkSessionLimit() error {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	return s.checkSessionLimitLocked()
}

func (s *serverState) checkSessionLimitLocked() error {
	if limit := s.cfg.MaxSessions; limit > 0 && len(s.sessions) >= limit {
		return fmt.Errorf("%w: at most %d sessions may exist at once", errSessionLimit, limit)
	}
	return nil
}

// newSessionID returns an unguessable session id. Session ids are the only
// handle on a session, so a client must not be able to predict another
// client's id.
func newSessionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "sess-" + hex.EncodeToString(raw[:]), nil
}

func (s *serverState) lookupSession(owner, id string) (*serverSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("%w: session_id must not be empty", errInvalidArgument)
	}

	s.reapExpiredSessions(time.Now().UTC())

	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	session, ok := s.sessions[id]
	if !ok || session.owner != owner {
		return nil, fmt.Errorf("%w: %s", errSessionNotFound, id)
	}
	return session, nil
}

// acquireSession looks up a session and marks it busy in one step under
// sessionsMu, so destroySession and reapExpiredSessions (which skip busy
// sessions under the same lock) cannot remove it before it runs. The caller
// must call exec on the returned session to release it.
func (s *serverState) acquireSession(owner, id string) (*serverSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("%w: session_id must not be empty", errInvalidArgument)
	}

	s.reapExpiredSessions(time.Now().UTC())

	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	session, ok := s.sessions[id]
	if !ok || session.owner != owner {
		return nil, fmt.Errorf("%w: %s", errSessionNotFound, id)
	}
	if err := session.markBusy(); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *serverState) listSessions(owner string) []sessionSummary {
	s.sessionsMu.Lock()
	sessions := make([]*serverSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		if session.owner == owner {
			sessions = append(sessions, session)
		}
	}
	s.sessionsMu.Unlock()

	// Session ids are random, so list in creation order.
	slices.SortFunc(sessions, func(a, b *serverSession) int {
		if c := a.createdAt.Compare(b.createdAt); c != 0 {
			return c
		}
		return strings.Compare(a.id, b.id)
	})
	out := make([]sessionSummary, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, session.summary())
	}
	return out
}

func (s *serverState) destroySession(owner, id string) (*serverSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("%w: session_id must not be empty", errInvalidArgument)
	}

	s.sessionsMu.Lock()
	session, ok := s.sessions[id]
	if !ok || session.owner != owner {
		s.sessionsMu.Unlock()
		return nil, fmt.Errorf("%w: %s", errSessionNotFound, id)
	}
	if session.isBusy() {
		s.sessionsMu.Unlock()
		return nil, fmt.Errorf("%w: %s", errSessionBusy, id)
	}
	delete(s.sessions, id)
	s.sessionsMu.Unlock()

	return session, nil
}

// dropOwnedSessions removes every session owned by a closed connection,
// since no other connection can reach them.
func (s *serverState) dropOwnedSessions(owner string) {
	s.sessionsMu.Lock()
	for id, session := range s.sessions {
		if session.owner == owner {
			delete(s.sessions, id)
		}
	}
	s.sessionsMu.Unlock()
}

func (s *serverState) reapExpiredSessions(now time.Time) {
	s.sessionsMu.Lock()
	for id, session := range s.sessions {
		if session.expired(now) {
			delete(s.sessions, id)
		}
	}
	s.sessionsMu.Unlock()
}

func (s *serverSession) summary() sessionSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.summaryLocked()
}

func (s *serverSession) summaryLocked() sessionSummary {
	summary := sessionSummary{
		SessionID: s.id,
		State:     "idle",
		CreatedAt: formatTime(s.createdAt),
		UpdatedAt: formatTime(s.updatedAt),
	}
	if s.busy {
		summary.State = "running"
		return summary
	}
	if !s.idleSince.IsZero() {
		summary.ExpiresAt = formatTime(s.idleSince.Add(s.server.cfg.SessionTTL))
	}
	return summary
}

func (s *serverSession) isBusy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

func (s *serverSession) expired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy || s.idleSince.IsZero() || s.server.cfg.SessionTTL <= 0 {
		return false
	}
	return !now.Before(s.idleSince.Add(s.server.cfg.SessionTTL))
}

func (s *serverSession) markBusy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return fmt.Errorf("%w: %s", errSessionBusy, s.id)
	}
	s.busy = true
	s.updatedAt = time.Now().UTC()
	s.idleSince = time.Time{}
	return nil
}

// exec runs a script on a session that the caller has already marked busy
// via acquireSession, and marks it idle again when the script finishes.
func (s *serverSession) exec(ctx context.Context, params *sessionExecParams) (*sessionExecResult, error) {
	if params == nil {
		params = &sessionExecParams{}
	}

	result, err := s.shell.Exec(ctx, &gbash.ExecutionRequest{
		Name:           params.Name,
		Script:         params.Script,
		ShellVariant:   params.ShellVariant,
		Args:           append([]string(nil), params.Args...),
		StartupOptions: append([]string(nil), params.StartupOptions...),
		Env:            copyStringMap(params.Env),
		WorkDir:        params.WorkDir,
		Timeout:        s.execTimeout(params.TimeoutMs),
		ReplaceEnv:     params.ReplaceEnv,
	})

	finished := time.Now().UTC()

	s.mu.Lock()
	s.busy = false
	s.updatedAt = finished
	s.idleSince = finished
	summary := s.summaryLocked()
	s.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("execute session script: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("execute session script: runtime returned no result")
	}

	var finalEnv map[string]string
	var finalEnvTruncated bool
	if params.IncludeFinalEnv {
		finalEnv, finalEnvTruncated = limitFinalEnv(result.FinalEnv, maxFinalEnvBytes)
	}

	return &sessionExecResult{
		SessionID:         s.id,
		ExitCode:          result.ExitCode,
		Stdout:            result.Stdout,
		Stderr:            result.Stderr,
		StdoutBase64:      base64IfNotUTF8(result.Stdout),
		StderrBase64:      base64IfNotUTF8(result.Stderr),
		StdoutTruncated:   result.StdoutTruncated,
		StderrTruncated:   result.StderrTruncated,
		FinalEnv:          finalEnv,
		FinalEnvTruncated: finalEnvTruncated,
		ShellExited:       result.ShellExited,
		ControlStderr:     strings.TrimSpace(result.ControlStderr),
		StartedAt:         formatTime(result.StartedAt),
		FinishedAt:        formatTime(result.FinishedAt),
		DurationMs:        durationMilliseconds(result.Duration),
		Session:           summary,
	}, nil
}

// limitFinalEnv copies env in name order, stopping before the copy would
// exceed limit bytes of names and values.
func limitFinalEnv(env map[string]string, limit int) (map[string]string, bool) {
	if len(env) == 0 {
		return nil, false
	}
	out := make(map[string]string, len(env))
	used := 0
	for _, name := range slices.Sorted(maps.Keys(env)) {
		value := env[name]
		used += len(name) + len(value)
		if used > limit {
			return out, true
		}
		out[name] = value
	}
	return out, false
}

// execTimeout applies the server's MaxExecTimeout cap to a requested
// timeout_ms value.
func (s *serverSession) execTimeout(ms int64) time.Duration {
	timeout := durationFromMillis(ms)
	if limit := s.server.cfg.MaxExecTimeout; limit > 0 && (timeout <= 0 || timeout > limit) {
		return limit
	}
	return timeout
}

// durationFromMillis converts a timeout_ms value, saturating instead of
// overflowing so a huge value never becomes a tiny or negative timeout.
func durationFromMillis(ms int64) time.Duration {
	if ms <= 0 {
		return 0
	}
	if ms > math.MaxInt64/int64(time.Millisecond) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(ms) * time.Millisecond
}

// base64IfNotUTF8 returns the base64 encoding of text when JSON encoding
// would otherwise replace invalid UTF-8 bytes, and "" when text is valid.
func base64IfNotUTF8(text string) string {
	if utf8.ValidString(text) {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(text))
}

func durationMilliseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func formatTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339Nano)
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	maps.Copy(out, src)
	return out
}
