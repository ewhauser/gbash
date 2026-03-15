package sqlite3

import (
	"context"
	"fmt"
	"strings"
	"testing"

	gbruntime "github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/internal/fuzztest"
	"github.com/ewhauser/gbash/policy"
)

const (
	fuzzMaxValueBytes = 2 << 10
)

func newFuzzRuntime(tb testing.TB) *gbruntime.Runtime {
	tb.Helper()
	return fuzztest.NewRuntime(tb, fuzztest.RuntimeOptions{
		Registry: newSQLiteRegistry(tb),
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:  []string{"/"},
			WriteRoots: []string{"/"},
			Limits: policy.Limits{
				MaxCommandCount:      200,
				MaxGlobOperations:    2000,
				MaxLoopIterations:    200,
				MaxSubstitutionDepth: 8,
				MaxStdoutBytes:       16 << 10,
				MaxStderrBytes:       16 << 10,
				MaxFileBytes:         128 << 10,
			},
		}),
	})
}

func newFuzzSession(tb testing.TB, rt *gbruntime.Runtime) *gbruntime.Session {
	tb.Helper()
	return fuzztest.NewSession(tb, rt)
}

func runFuzzSessionScript(t *testing.T, session *gbruntime.Session, script []byte) (*gbruntime.ExecutionResult, error) {
	t.Helper()
	return fuzztest.RunSessionScript(t, session, "sqlite-fuzz.sh", script)
}

func warmFuzzSQLite(tb testing.TB, rt *gbruntime.Runtime) {
	tb.Helper()

	session := newFuzzSession(tb, rt)
	result, err := session.Exec(context.Background(), &gbruntime.ExecutionRequest{
		Name:    "sqlite-warmup.sh",
		Script:  "sqlite3 :memory: \"select 1;\"\n",
		Timeout: fuzztest.DefaultWarmupTimeout,
	})
	if err != nil {
		tb.Fatalf("sqlite fuzz warmup error = %v", err)
	}
	if result == nil {
		tb.Fatalf("sqlite fuzz warmup returned nil result")
		return
	}
	if result.ExitCode != 0 {
		tb.Fatalf("sqlite fuzz warmup ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
}

func assertSuccessfulFuzzExecution(t *testing.T, script []byte, result *gbruntime.ExecutionResult, err error) {
	t.Helper()
	fuzztest.AssertSuccessfulFuzzExecution(t, script, result, err)
}

func sanitizeSQLiteValue(raw string) string {
	raw = strings.ToValidUTF8(raw, "?")
	raw = strings.ReplaceAll(raw, "\x00", "?")
	if len(raw) > fuzzMaxValueBytes {
		raw = raw[:fuzzMaxValueBytes]
	}
	return raw
}

func sqliteStringLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func FuzzSQLiteCommands(f *testing.F) {
	rt := newFuzzRuntime(f)
	warmFuzzSQLite(f, rt)

	for _, seed := range []string{"alpha", "beta,gamma", "O'Reilly", ""} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		session := newFuzzSession(t, rt)
		value := sanitizeSQLiteValue(raw)
		sql := fmt.Sprintf(
			"create table t(value text); insert into t values ('%s'); select value from t;",
			sqliteStringLiteral(value),
		)

		writeSessionFile(t, session, "/tmp/query.sql", []byte(sql+"\n"))
		script := []byte("sqlite3 :memory: </tmp/query.sql >/tmp/sqlite-value.txt\n")
		result, err := runFuzzSessionScript(t, session, script)
		assertSuccessfulFuzzExecution(t, script, result, err)
	})
}

func FuzzSQLiteFileCommands(f *testing.F) {
	rt := newFuzzRuntime(f)
	warmFuzzSQLite(f, rt)

	for _, seed := range []string{"alpha", "beta,gamma", "O'Reilly", ""} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		session := newFuzzSession(t, rt)
		value := sanitizeSQLiteValue(raw)
		sql := fmt.Sprintf(
			"create table if not exists items(value text); insert into items values ('%s'); select value from items order by value;",
			sqliteStringLiteral(value),
		)

		writeSessionFile(t, session, "/tmp/query.sql", []byte(sql+"\n"))
		script := []byte("sqlite3 -json /tmp/data.db </tmp/query.sql >/tmp/sqlite-json.txt\n")
		result, err := runFuzzSessionScript(t, session, script)
		assertSuccessfulFuzzExecution(t, script, result, err)
	})
}
