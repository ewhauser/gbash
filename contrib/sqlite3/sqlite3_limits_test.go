package sqlite3

import (
	"context"
	"strings"
	"testing"
	"time"

	gbruntime "github.com/ewhauser/gbash"
)

const sqliteInfiniteRowsSQL = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c) SELECT x FROM c;`

// execSessionWithTimeout runs script with a request timeout and fails the test
// if the execution does not return well after that timeout has elapsed.
func execSessionWithTimeout(tb testing.TB, session *gbruntime.Session, script string, timeout time.Duration) *gbruntime.ExecutionResult {
	tb.Helper()

	type outcome struct {
		result *gbruntime.ExecutionResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := session.Exec(context.Background(), &gbruntime.ExecutionRequest{Script: script, Timeout: timeout})
		done <- outcome{result: result, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			tb.Fatalf("Session.Exec() error = %v", got.err)
		}
		return got.result
	case <-time.After(timeout + 10*time.Second):
		tb.Fatalf("Session.Exec() did not return within %s of its %s timeout", 10*time.Second, timeout)
		return nil
	}
}

func TestSQLite3InterruptsLongRunningQueryOnTimeout(t *testing.T) {
	t.Parallel()
	session := newSQLiteSession(t)

	result := execSessionWithTimeout(t, session, `sqlite3 :memory: "WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c) SELECT count(*) FROM c;"`+"\n", 200*time.Millisecond)

	if result.ExitCode != 124 {
		t.Fatalf("ExitCode = %d, want 124; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "execution timed out") {
		t.Fatalf("Stderr = %q, want timeout message", result.Stderr)
	}
}

func TestSQLite3StreamsRowsBeforeStatementCompletes(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"-list", "-csv", "-json", "-line", "-markdown", "-tabs", "-quote", "-html", "-ascii"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			session := newSQLiteSession(t)

			result := execSessionWithTimeout(t, session, `sqlite3 `+mode+` :memory: "`+sqliteInfiniteRowsSQL+`"`+"\n", 300*time.Millisecond)

			if result.ExitCode != 124 {
				t.Fatalf("ExitCode = %d, want 124; stderr=%q", result.ExitCode, result.Stderr)
			}
			if result.Stdout == "" {
				t.Fatalf("Stdout is empty; rows were buffered instead of streamed")
			}
		})
	}
}

func TestSQLite3StreamingOutputMatchesBufferedFormatting(t *testing.T) {
	t.Parallel()
	session := newSQLiteSession(t)

	const setup = `create table t(id integer, name text); insert into t values (1, 'a'), (2, null), (3, 'c'); `
	cases := []struct {
		args string
		sql  string
		want string
	}{
		{args: "-json", sql: "select id, name from t order by id;", want: `[{"id":1,"name":"a"},{"id":2,"name":null},{"id":3,"name":"c"}]` + "\n"},
		{args: "-json", sql: "select id from t where id > 10;", want: ""},
		{args: "-line", sql: "select id, name from t order by id;", want: "id   = 1\nname = a\n\nid   = 2\nname = \n\nid   = 3\nname = c\n"},
		{args: "-csv -header", sql: "select id from t where id > 10;", want: "id\n"},
		{args: "-csv", sql: "select id from t where id > 10;", want: ""},
		{args: "-header", sql: "select id from t where id > 10;", want: "id\n"},
		{args: "-markdown -header", sql: "select id, name from t order by id limit 1;", want: "| id | name |\n|---|---|\n| 1 | a |\n"},
		{args: "-markdown", sql: "select id from t where id > 10;", want: ""},
	}
	for _, tc := range cases {
		result := mustExecSession(t, session, `sqlite3 `+tc.args+` :memory: "`+setup+tc.sql+`"`+"\n")
		if result.ExitCode != 0 {
			t.Fatalf("%s %q: ExitCode = %d, want 0; stderr=%q", tc.args, tc.sql, result.ExitCode, result.Stderr)
		}
		if result.Stdout != tc.want {
			t.Fatalf("%s %q: Stdout = %q, want %q", tc.args, tc.sql, result.Stdout, tc.want)
		}
	}
}

func TestSQLite3BoundsBufferedColumnModes(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"-column", "-table", "-box"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			session := newSQLiteSession(t)

			result := execSessionWithTimeout(t, session, `sqlite3 `+mode+` :memory: "`+sqliteInfiniteRowsSQL+`"`+"\n", 30*time.Second)

			if result.ExitCode != 1 {
				t.Fatalf("ExitCode = %d, want 1; stderr=%q", result.ExitCode, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "result too large") {
				t.Fatalf("Stderr = %q, want result-too-large diagnostic", result.Stderr)
			}
		})
	}
}

// Temp tables must never spill to the host temp directory. Point the host
// temp directory somewhere unusable so any host temp file creation fails.
func TestSQLite3KeepsTempStorageOffHostFilesystem(t *testing.T) {
	t.Setenv("SQLITE_TMPDIR", "/nonexistent/gbash-sqlite3-temp")
	session := newSQLiteSession(t)

	result := mustExecSession(t, session, `sqlite3 :memory: "create temp table t(x); insert into t select randomblob(4096) from (WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM c WHERE i < 2000) SELECT i FROM c); select count(*) from t; select count(*) from (select x from t order by x); pragma temp_store;"`+"\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "2000\n2000\n2\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}

	pragma := mustExecSession(t, session, `sqlite3 :memory: "pragma temp_store=FILE;"`+"\n")
	if pragma.ExitCode == 0 {
		t.Fatalf("pragma temp_store=FILE ExitCode = 0, want failure; stdout=%q", pragma.Stdout)
	}
}

func TestSQLite3BoundsInMemoryStorage(t *testing.T) {
	t.Parallel()
	session := newSQLiteSession(t)

	result := execSessionWithTimeout(t, session, `sqlite3 :memory: "create temp table t(x); insert into t select randomblob(1048576) from (WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM c WHERE i < 4096) SELECT i FROM c);"`+"\n", 30*time.Second)

	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "out of memory") {
		t.Fatalf("Stderr = %q, want out-of-memory diagnostic", result.Stderr)
	}
}
