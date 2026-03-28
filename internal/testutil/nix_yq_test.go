package testutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveNixYQ(t *testing.T) {
	t.Run("missing env", func(t *testing.T) {
		t.Setenv(nixYQEnv, "")

		_, _, err := resolveNixYQ(context.Background())
		if !errors.Is(err, errNixYQUnset) {
			t.Fatalf("resolveNixYQ() error = %v, want %v", err, errNixYQUnset)
		}
	})

	t.Run("wrong version", func(t *testing.T) {
		path := writeFakeYQ(t, "yq (https://github.com/mikefarah/yq/) version v4.52.5")
		t.Setenv(nixYQEnv, path)

		_, _, err := resolveNixYQ(context.Background())
		if err == nil {
			t.Fatal("resolveNixYQ() error = nil, want version error")
		}
		if !strings.Contains(err.Error(), "tests require yq "+pinnedNixYQVersion) {
			t.Fatalf("resolveNixYQ() error = %v, want pinned version diagnostic", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		wantFirstLine := "yq (https://github.com/mikefarah/yq/) version v4.52.4"
		path := writeFakeYQ(t, wantFirstLine)
		t.Setenv(nixYQEnv, path)

		gotPath, gotFirstLine, err := resolveNixYQ(context.Background())
		if err != nil {
			t.Fatalf("resolveNixYQ() error = %v", err)
		}
		if gotPath != path {
			t.Fatalf("resolveNixYQ() path = %q, want %q", gotPath, path)
		}
		if gotFirstLine != wantFirstLine {
			t.Fatalf("resolveNixYQ() firstLine = %q, want %q", gotFirstLine, wantFirstLine)
		}
	})
}

func writeFakeYQ(t *testing.T, firstLine string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "yq")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' " + shellQuote(firstLine) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}
