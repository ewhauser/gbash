package runtime

import (
	"context"
	"errors"
	"fmt"
	stdfs "io/fs"
	"testing"

	"github.com/ewhauser/gbash/commands"
	"github.com/ewhauser/gbash/internal/builtins"
)

func TestRedirectIntoMissingDirectoryFailsWithoutCreatingParents(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session, ""+
		"echo hi > /tmp/nope/dir/x; echo \"rc=$?\"\n"+
		"echo hi >> /tmp/nope/x; echo \"rc=$?\"\n"+
		"if [ -e /tmp/nope ]; then echo created; else echo absent; fi\n"+
		"touch /tmp/file\n"+
		"echo hi > /tmp/file/x; echo \"rc=$?\"\n"+
		"echo hi > /tmp/ok; cat /tmp/ok\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "rc=1\nrc=1\nabsent\nrc=1\nhi\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	wantStderr := "" +
		"/tmp/nope/dir/x: No such file or directory\n" +
		"/tmp/nope/x: No such file or directory\n" +
		"/tmp/file/x: Not a directory\n"
	if got := result.Stderr; got != wantStderr {
		t.Fatalf("Stderr = %q, want %q", got, wantStderr)
	}
}

func TestCustomCommandRenameIntoOwnSubdirectoryReturnsError(t *testing.T) {
	t.Parallel()
	registry := builtins.DefaultRegistry()
	if err := registry.Register(commands.DefineCommand("renameprobe", func(ctx context.Context, inv *commands.Invocation) error {
		err := inv.FS.Rename(ctx, inv.Args[0], inv.Args[1])
		switch {
		case err == nil:
			_, err = fmt.Fprintln(inv.Stdout, "renamed")
		case errors.Is(err, stdfs.ErrInvalid):
			_, err = fmt.Fprintln(inv.Stdout, "einval")
		case errors.Is(err, stdfs.ErrNotExist):
			_, err = fmt.Fprintln(inv.Stdout, "enoent")
		}
		return err
	})); err != nil {
		t.Fatalf("Register(renameprobe) error = %v", err)
	}

	session := newSession(t, &Config{Registry: registry})
	result := mustExecSession(t, session, ""+
		"mkdir -p /tmp/a/b && echo keep > /tmp/a/b/file\n"+
		"renameprobe /tmp/a /tmp/a/b/c\n"+
		"renameprobe /tmp/a /tmp/missing/c\n"+
		"cat /tmp/a/b/file\n"+
		"renameprobe /tmp/a /tmp/moved && cat /tmp/moved/b/file\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "einval\nenoent\nkeep\nrenamed\nkeep\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}
