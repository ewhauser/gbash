package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"math"
	"strings"
	"testing"
	"time"
)

func TestDurationFromMillisSaturates(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		ms   int64
		want time.Duration
	}{
		{ms: -1, want: 0},
		{ms: 0, want: 0},
		{ms: 1500, want: 1500 * time.Millisecond},
		{ms: math.MaxInt64 / int64(time.Millisecond), want: time.Duration(math.MaxInt64/int64(time.Millisecond)) * time.Millisecond},
		{ms: 18446744073710, want: time.Duration(math.MaxInt64)},
		{ms: math.MaxInt64, want: time.Duration(math.MaxInt64)},
	} {
		if got := durationFromMillis(tc.ms); got != tc.want {
			t.Fatalf("durationFromMillis(%d) = %v, want %v", tc.ms, got, tc.want)
		}
	}
}

func TestInternalErrorsAreLoggedNotReturned(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	srv := newTestServerState(t, time.Minute)
	srv.errorLog = log.New(&logs, "", 0)

	secret := errors.New("open /Users/someone/private/gbash-root/x: permission denied")
	resp := srv.appErrorResponse(json.RawMessage(`"req-1"`), secret)
	if resp.Error == nil || resp.Error.Code != rpcInternalError || resp.Error.Message != "internal error" {
		t.Fatalf("response error = %+v, want generic internal error", resp.Error)
	}
	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "/Users/someone") {
		t.Fatalf("response leaks host path: %s", encoded)
	}
	if !strings.Contains(logs.String(), "/Users/someone/private") {
		t.Fatalf("logs = %q, want internal error details", logs.String())
	}
}

func TestParseRPCRequestNotifications(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		raw          string
		notification bool
		wantErr      bool
	}{
		{raw: `{"jsonrpc":"2.0","method":"system.ping"}`, notification: true},
		{raw: `{"jsonrpc":"2.0","id":null,"method":"system.ping"}`},
		{raw: `{"jsonrpc":"2.0","id":7,"method":"system.ping"}`},
		{raw: `{"jsonrpc":"2.0","id":{},"method":"system.ping"}`, wantErr: true},
		{raw: `{"jsonrpc":"2.0","method":""}`, wantErr: true},
		{raw: `{"jsonrpc":"1.0","method":"system.ping"}`, wantErr: true},
	} {
		_, notification, errResp := parseRPCRequest(json.RawMessage(tc.raw))
		if (errResp != nil) != tc.wantErr {
			t.Fatalf("parseRPCRequest(%s) error response = %+v, wantErr %t", tc.raw, errResp, tc.wantErr)
		}
		if notification != tc.notification {
			t.Fatalf("parseRPCRequest(%s) notification = %t, want %t", tc.raw, notification, tc.notification)
		}
	}
}
