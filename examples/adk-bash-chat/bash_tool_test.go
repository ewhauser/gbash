package main

import (
	"context"
	"strings"
	"testing"
)

func TestSeedLabCreatesFixturesAndSQLiteDatabase(t *testing.T) {
	t.Parallel()

	tool, err := newPersistentBashTool(context.Background())
	if err != nil {
		t.Fatalf("newPersistentBashTool() error = %v", err)
	}

	first, err := tool.runScript(context.Background(), bashToolInput{
		Script: "test -f /home/agent/lab/README.md && test -f /home/agent/lab/incidents.db && sqlite3 /home/agent/lab/incidents.db 'select count(*) from incidents;'",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if first.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", first.ExitCode, first.Stderr)
	}
	if strings.TrimSpace(first.Stdout) != "4" {
		t.Fatalf("stdout = %q, want %q", first.Stdout, "4\n")
	}
}

func TestPersistentBashToolCarriesWorkDirAndEnv(t *testing.T) {
	t.Parallel()

	tool, err := newPersistentBashTool(context.Background())
	if err != nil {
		t.Fatalf("newPersistentBashTool() error = %v", err)
	}

	first, err := tool.runScript(context.Background(), bashToolInput{
		Script: "cd /home/agent/work\nexport REPORT_NAME=summary.md\npwd\n",
	})
	if err != nil {
		t.Fatalf("Run(first) error = %v", err)
	}
	if first.ExitCode != 0 {
		t.Fatalf("first exit = %d, stderr = %q", first.ExitCode, first.Stderr)
	}
	if strings.TrimSpace(first.Stdout) != "/home/agent/work" {
		t.Fatalf("first stdout = %q", first.Stdout)
	}
	if first.PWD != "/home/agent/work" {
		t.Fatalf("first pwd = %q, want %q", first.PWD, "/home/agent/work")
	}

	second, err := tool.runScript(context.Background(), bashToolInput{
		Script: "printf '%s %s\\n' \"$PWD\" \"$REPORT_NAME\"",
	})
	if err != nil {
		t.Fatalf("Run(second) error = %v", err)
	}
	if second.ExitCode != 0 {
		t.Fatalf("second exit = %d, stderr = %q", second.ExitCode, second.Stderr)
	}
	if got, want := strings.TrimSpace(second.Stdout), "/home/agent/work summary.md"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
