package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestPrintfReusesFormatAndHonorsStop(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf '%s:%b' one 'a\\n' two 'b\\cignored' three 'c\\n'\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "one:a\ntwo:b"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestCommSupportsStdinAndColumnSuppression(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'apple\\nbanana\\ncarrot\\n' > /tmp/left.txt\nprintf 'banana\\ndate\\n' > /tmp/right.txt\ncat /tmp/left.txt | comm -23 - /tmp/right.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "apple\ncarrot\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestPasteSupportsRepeatedStdin(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'a\\nb\\nc\\nd\\n' | paste - -\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "a\tb\nc\td\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestPasteSerialSupportsCustomDelimiter(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'one\\ntwo\\nthree\\n' > /tmp/in.txt\npaste -s -d ',' /tmp/in.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "one,two,three\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestTRTranslatesRanges(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'abcxyz\\n' | tr 'a-z' 'A-Z'\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "ABCXYZ\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestTRSupportsComplementDelete(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'abc123xyz\\n' | tr -cd '[:digit:]'\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "123"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestRevHandlesUnicode(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'abé\\nmañana\\n' | rev\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "éba\nanañam\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestNLSupportsFormattingControls(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'one\\n\\ntwo\\n' | nl -ba -v 10 -i 5 -w 3 -s ': '\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, " 10: one\n 15: \n 20: two\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestJoinSupportsCustomOutputAndUnpairedLines(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'a,1\\nb,2\\n' > /tmp/left.csv\nprintf 'a,x\\nc,y\\n' > /tmp/right.csv\njoin -t , -a1 -e NA -o 0,1.2,2.2 /tmp/left.csv /tmp/right.csv\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "a,1,x\nb,2,NA\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestSplitWritesNumberedChunks(t *testing.T) {
	session := newSession(t, &Config{})
	writeSessionFile(t, session, "/tmp/in.txt", []byte("one\ntwo\nthree\nfour\nfive\n"))

	result, err := session.Exec(context.Background(), &ExecutionRequest{
		Script: "split -l 2 -d -a 2 /tmp/in.txt /tmp/out-\n",
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	if got, want := string(readSessionFile(t, session, "/tmp/out-00")), "one\ntwo\n"; got != want {
		t.Fatalf("out-00 = %q, want %q", got, want)
	}
	if got, want := string(readSessionFile(t, session, "/tmp/out-01")), "three\nfour\n"; got != want {
		t.Fatalf("out-01 = %q, want %q", got, want)
	}
	if got, want := string(readSessionFile(t, session, "/tmp/out-02")), "five\n"; got != want {
		t.Fatalf("out-02 = %q, want %q", got, want)
	}
}

func TestTacReversesLineOrder(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'one\\ntwo\\nthree\\n' | tac\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "three\ntwo\none\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestDiffEmitsUnifiedDiff(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'one\\ntwo\\n' > /tmp/a.txt\nprintf 'one\\nTHREE\\n' > /tmp/b.txt\ndiff -u /tmp/a.txt /tmp/b.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; stderr=%q", result.ExitCode, result.Stderr)
	}
	for _, want := range []string{"--- /tmp/a.txt", "+++ /tmp/b.txt", "-two", "+THREE"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("Stdout = %q, want %q", result.Stdout, want)
		}
	}
}

func TestDiffReportsIdenticalFiles(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'same\\n' > /tmp/a.txt\ndiff -s /tmp/a.txt /tmp/a.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "Files /tmp/a.txt and /tmp/a.txt are identical\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestBase64RoundTripsThroughPipelines(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'hello world' | base64 -w 0 | base64 -d\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "hello world"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestBase64DecodeIgnoresWhitespace(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'aGVs\\nbG8=\\n' | base64 -d\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "hello"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}
