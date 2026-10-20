package builtins_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTextBuiltinReviewFixes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		script     string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "xan fixlengths rejects huge width", script: "printf 'a,b\\n1,2\\n' | xan fixlengths -l 2000000000\n", wantCode: 1, wantStderr: "field width too large"},
		{name: "xan fixlengths charges cell budget", script: "yes 1 | head -n 100 > /tmp/in.csv\nxan fixlengths -l 1000000 /tmp/in.csv\n", wantCode: 1, wantStderr: "output too large"},
		{name: "xan fixlengths rejects negative width", script: "printf 'a,b\\n' | xan fixlengths -l -1\n", wantCode: 1, wantStderr: "invalid length"},
		{name: "xan shuffle seed with unit step", script: "printf 'h\\n1\\n2\\n3\\n' | xan shuffle --seed 230538014 | sort\n", wantStdout: "1\n2\n3\nh\n"},
		{name: "xan sample seed with unit step", script: "printf 'h\\n1\\n2\\n3\\n' | xan sample --seed 230538014 3 | wc -l\n", wantStdout: "4\n"},
		{name: "factor rejects oversized input", script: "factor 1" + strings.Repeat("0", 200) + "1\n", wantCode: 1, wantStderr: "too large"},
		{name: "rev keeps invalid bytes", script: "printf 'ab\\377cd\\n' | rev | od -An -c | tr -s ' '\n", wantStdout: " d c 377 b a \\n\n"},
		{name: "sed rejects undefined group", script: "echo abc | sed 's/b/\\9/'\n", wantCode: 1, wantStderr: "invalid reference \\9"},
		{name: "sed allows defined group", script: "echo abc | sed 's/\\(b\\)/[\\1]/'\n", wantStdout: "a[b]c\n"},
		{name: "seq rejects huge width", script: "seq -f '%1000000000f' 1 1\n", wantCode: 1, wantStderr: "field width too large"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := newRuntime(t, &Config{})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			result, err := rt.Run(ctx, &ExecutionRequest{Script: tc.script})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.ExitCode != tc.wantCode {
				t.Fatalf("ExitCode = %d, want %d; stdout=%q stderr=%q", result.ExitCode, tc.wantCode, result.Stdout, result.Stderr)
			}
			if tc.wantStdout != "" && result.Stdout != tc.wantStdout {
				t.Fatalf("Stdout = %q, want %q", result.Stdout, tc.wantStdout)
			}
			if !strings.Contains(result.Stderr, tc.wantStderr) {
				t.Fatalf("Stderr = %q, want substring %q", result.Stderr, tc.wantStderr)
			}
		})
	}
}

func TestCurlRepeatedAndFileData(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
	}))
	defer server.Close()
	rt := newRuntime(t, &Config{Network: &NetworkConfig{
		AllowedURLPrefixes: []string{server.URL},
		AllowedMethods:     []Method{MethodGet, MethodHead, MethodPost},
	}})
	run := func(script string) *ExecutionResult {
		t.Helper()
		result, err := rt.Run(context.Background(), &ExecutionRequest{Script: script})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		return result
	}
	if got := run("curl -s -d a=1 -d b=2 " + server.URL + "\n"); got.Stdout != "a=1&b=2" {
		t.Fatalf("repeated -d Stdout = %q (stderr %q)", got.Stdout, got.Stderr)
	}
	if got := run("printf 'x=1\\ny=2\\n' > /tmp/d\ncurl -s -d @/tmp/d " + server.URL + "\n"); got.Stdout != "x=1y=2" {
		t.Fatalf("-d @file Stdout = %q (stderr %q)", got.Stdout, got.Stderr)
	}
	if got := run("printf 'x=1\\ny=2\\n' > /tmp/d\ncurl -s --data-binary @/tmp/d " + server.URL + "\n"); got.Stdout != "x=1\ny=2\n" {
		t.Fatalf("--data-binary @file Stdout = %q (stderr %q)", got.Stdout, got.Stderr)
	}
	if got := run("curl -s --data-raw @/tmp/d " + server.URL + "\n"); got.Stdout != "@/tmp/d" {
		t.Fatalf("--data-raw Stdout = %q (stderr %q)", got.Stdout, got.Stderr)
	}
	if got := run("curl -s -F f=@/tmp/missing " + server.URL + "\n"); got.ExitCode != 26 {
		t.Fatalf("missing -F file ExitCode = %d, want 26 (stderr %q)", got.ExitCode, got.Stderr)
	}
}
