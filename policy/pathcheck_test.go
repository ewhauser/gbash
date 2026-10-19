package policy

import (
	"context"
	"os"
	"strings"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
)

func newPathCheckFS(t *testing.T) gbfs.FileSystem {
	t.Helper()
	ctx := context.Background()
	mem := gbfs.NewMemory()
	for _, dir := range []string{"/outside/keep", "/home/agent/real"} {
		if err := mem.MkdirAll(ctx, dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}
	file, err := mem.OpenFile(ctx, "/outside/keep/victim", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(victim) error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(victim) error = %v", err)
	}
	for linkName, target := range map[string]string{
		"/home/agent/link":        "/outside",
		"/home/agent/dangling":    "/outside/newfile",
		"/home/agent/danglingdir": "../../outside/newdir",
		"/home/agent/inner":       "real",
	} {
		if err := mem.Symlink(ctx, target, linkName); err != nil {
			t.Fatalf("Symlink(%q) error = %v", linkName, err)
		}
	}
	return mem
}

func TestCheckPathResolvesMutationsThroughSymlinkedParents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action FileAction
		target string
		allow  map[SymlinkMode]bool
	}{
		{name: "write under symlink into missing dir", action: FileActionWrite, target: "/home/agent/link/new/f"},
		{name: "write through dangling symlink", action: FileActionWrite, target: "/home/agent/dangling"},
		{name: "write under dangling dir symlink", action: FileActionWrite, target: "/home/agent/danglingdir/f"},
		{name: "mkdir under symlink into missing dirs", action: FileActionMkdir, target: "/home/agent/link/a/b"},
		{name: "remove under symlinked parent", action: FileActionRemove, target: "/home/agent/link/keep"},
		{name: "rename under symlinked parent", action: FileActionRename, target: "/home/agent/link/keep/victim"},
		{
			name:   "remove symlink entry itself",
			action: FileActionRemove,
			target: "/home/agent/link",
			allow:  map[SymlinkMode]bool{SymlinkDeny: true, SymlinkFollow: true},
		},
		{
			name:   "write into missing dirs without symlinks",
			action: FileActionWrite,
			target: "/home/agent/new/dir/f",
			allow:  map[SymlinkMode]bool{SymlinkDeny: true, SymlinkFollow: true},
		},
		{
			name:   "write through in-root symlink",
			action: FileActionWrite,
			target: "/home/agent/inner/new/f",
			allow:  map[SymlinkMode]bool{SymlinkFollow: true},
		},
	}

	for _, mode := range []SymlinkMode{SymlinkDeny, SymlinkFollow} {
		pol := NewStatic(&Config{WriteRoots: []string{"/home/agent"}, SymlinkMode: mode})
		for _, tt := range tests {
			t.Run(string(mode)+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				err := CheckPath(context.Background(), pol, newPathCheckFS(t), tt.action, tt.target)
				if tt.allow[mode] {
					if err != nil {
						t.Fatalf("CheckPath(%s %q) error = %v, want nil", tt.action, tt.target, err)
					}
					return
				}
				if !IsDenied(err) {
					t.Fatalf("CheckPath(%s %q) error = %v, want denial", tt.action, tt.target, err)
				}
				wantReason := "outside allowed roots"
				if mode == SymlinkDeny {
					wantReason = "symlink traversal denied"
				}
				if !strings.Contains(err.Error(), wantReason) {
					t.Fatalf("CheckPath(%s %q) error = %v, want %q", tt.action, tt.target, err, wantReason)
				}
			})
		}
	}
}
