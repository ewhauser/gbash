package server

import (
	"errors"
	"testing"
	"time"

	"github.com/ewhauser/gbash"
)

func newTestServerState(t *testing.T, ttl time.Duration) *serverState {
	t.Helper()
	rt, err := gbash.New()
	if err != nil {
		t.Fatalf("gbash.New() error = %v", err)
	}
	return &serverState{
		ctx:       t.Context(),
		cfg:       normalizeConfig(Config{Runtime: rt, Name: "gbash", Version: "test", SessionTTL: ttl}),
		transport: "unix",
		sessions:  make(map[string]*serverSession),
		conns:     make(map[string]*clientConn),
	}
}

func TestAcquiredSessionCannotBeDestroyedOrReaped(t *testing.T) {
	t.Parallel()
	srv := newTestServerState(t, time.Minute)

	session, err := srv.createSession("conn-test")
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	acquired, err := srv.acquireSession("conn-test", session.id)
	if err != nil {
		t.Fatalf("acquireSession(%q) error = %v", session.id, err)
	}
	if _, err := srv.acquireSession("conn-test", session.id); !errors.Is(err, errSessionBusy) {
		t.Fatalf("second acquireSession(%q) error = %v, want errSessionBusy", session.id, err)
	}
	if _, err := srv.destroySession("conn-test", session.id); !errors.Is(err, errSessionBusy) {
		t.Fatalf("destroySession(%q) error = %v, want errSessionBusy", session.id, err)
	}
	srv.reapExpiredSessions(time.Now().Add(time.Hour))
	if _, err := srv.lookupSession("conn-test", session.id); err != nil {
		t.Fatalf("lookupSession(%q) after reap error = %v, want session kept", session.id, err)
	}

	result, err := acquired.exec(t.Context(), &sessionExecParams{Script: "printf ok"})
	if err != nil {
		t.Fatalf("exec() error = %v", err)
	}
	if result.Stdout != "ok" || result.Session.State != "idle" {
		t.Fatalf("exec() = %+v, want stdout ok and idle session", result)
	}

	if _, err := srv.destroySession("conn-test", session.id); err != nil {
		t.Fatalf("destroySession(%q) after exec error = %v", session.id, err)
	}
}

// TestConcurrentDestroyAndExec races session.exec against session.destroy.
// Exactly one may win: either destroy removes the session and exec reports
// not found, or exec runs and destroy reports busy.
func TestConcurrentDestroyAndExec(t *testing.T) {
	t.Parallel()
	srv := newTestServerState(t, time.Minute)

	for range 200 {
		session, err := srv.createSession("conn-test")
		if err != nil {
			t.Fatalf("createSession() error = %v", err)
		}

		start := make(chan struct{})
		execErr := make(chan error, 1)
		destroyErr := make(chan error, 1)
		go func() {
			<-start
			acquired, err := srv.acquireSession("conn-test", session.id)
			if err != nil {
				execErr <- err
				return
			}
			// Destroy must not succeed while the session is acquired.
			srv.sessionsMu.Lock()
			_, ok := srv.sessions[session.id]
			srv.sessionsMu.Unlock()
			if !ok {
				execErr <- errors.New("acquired session missing from registry")
				return
			}
			_, err = acquired.exec(t.Context(), &sessionExecParams{Script: "true"})
			execErr <- err
		}()
		go func() {
			<-start
			_, err := srv.destroySession("conn-test", session.id)
			destroyErr <- err
		}()
		close(start)

		eErr, dErr := <-execErr, <-destroyErr
		switch {
		case dErr == nil:
			if !errors.Is(eErr, errSessionNotFound) && eErr != nil {
				t.Fatalf("exec error = %v after successful destroy, want nil or errSessionNotFound", eErr)
			}
		case errors.Is(dErr, errSessionBusy):
			if eErr != nil {
				t.Fatalf("exec error = %v while destroy reported busy", eErr)
			}
			if _, err := srv.destroySession("conn-test", session.id); err != nil {
				t.Fatalf("destroySession(%q) cleanup error = %v", session.id, err)
			}
		default:
			t.Fatalf("destroySession error = %v, want nil or errSessionBusy", dErr)
		}
	}
}
