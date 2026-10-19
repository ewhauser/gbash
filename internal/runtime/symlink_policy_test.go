package runtime

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
	"github.com/ewhauser/gbash/policy"
)

type symlinkFSFactory struct {
	files    map[string]string
	symlinks map[string]string
}

func (f symlinkFSFactory) New(ctx context.Context) (gbfs.FileSystem, error) {
	mem := gbfs.NewMemory()
	for name, contents := range f.files {
		if err := mem.MkdirAll(ctx, path.Dir(name), 0o755); err != nil {
			return nil, err
		}
		file, err := mem.OpenFile(ctx, name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write([]byte(contents)); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	}
	for linkName, target := range f.symlinks {
		if err := mem.MkdirAll(ctx, path.Dir(linkName), 0o755); err != nil {
			return nil, err
		}
		if err := mem.Symlink(context.Background(), target, linkName); err != nil { //nolint:contextcheck // test setup
			return nil, err
		}
	}
	return mem, nil
}

func TestDefaultPolicyDeniesSymlinkTraversal(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{
		FileSystem: CustomFileSystem(symlinkFSFactory{
			files: map[string]string{
				"/safe/target.txt": "hello\n",
			},
			symlinks: map[string]string{
				"/safe/link.txt": "target.txt",
			},
		}, defaultHomeDir),
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "cat /safe/link.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "symlink traversal denied") {
		t.Fatalf("Stderr = %q, want symlink denial", result.Stderr)
	}
}

func TestFollowModeChecksResolvedReadTargetAgainstAllowedRoots(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{
		FileSystem: CustomFileSystem(symlinkFSFactory{
			files: map[string]string{
				"/denied/secret.txt": "secret\n",
			},
			symlinks: map[string]string{
				"/safe/link.txt": "/denied/secret.txt",
			},
		}, defaultHomeDir),
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:   []string{"/safe", "/usr/bin", "/bin"},
			WriteRoots:  []string{"/safe"},
			SymlinkMode: policy.SymlinkFollow,
		}),
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "cat /safe/link.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, `read "/denied/secret.txt" denied`) {
		t.Fatalf("Stderr = %q, want resolved-target denial", result.Stderr)
	}
}

func TestFollowModeAllowsSymlinkTraversalWithinAllowedRoots(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{
		FileSystem: CustomFileSystem(symlinkFSFactory{
			files: map[string]string{
				"/safe/target.txt": "hello\n",
			},
			symlinks: map[string]string{
				"/safe/link.txt": "target.txt",
			},
		}, defaultHomeDir),
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:   []string{"/safe", "/usr/bin", "/bin"},
			WriteRoots:  []string{"/safe"},
			SymlinkMode: policy.SymlinkFollow,
		}),
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "cat /safe/link.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "hello\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestFollowModeChecksResolvedWriteTargetAgainstAllowedRoots(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{
		FileSystem: CustomFileSystem(symlinkFSFactory{
			files: map[string]string{
				"/denied/.keep": "",
			},
			symlinks: map[string]string{
				"/safe/out": "/denied",
			},
		}, defaultHomeDir),
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:   []string{"/safe", "/usr/bin", "/bin"},
			WriteRoots:  []string{"/safe"},
			SymlinkMode: policy.SymlinkFollow,
		}),
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "echo hi > /safe/out/new.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, `write "/denied/new.txt" denied`) {
		t.Fatalf("Stderr = %q, want resolved-write denial", result.Stderr)
	}
}

func TestSymlinkedParentMutationsStayInsideWriteRoots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		check  string
	}{
		{
			name:   "redirect into missing dir under symlink",
			script: "echo pwn > /safe/out/new/f\n",
			check:  "[ -e /denied/new ] && echo escaped || echo contained\n",
		},
		{
			name:   "redirect through dangling symlink",
			script: "echo pwn > /safe/dangling\n",
			check:  "[ -e /denied/created ] && echo escaped || echo contained\n",
		},
		{
			name:   "mkdir -p under symlink",
			script: "mkdir -p /safe/out/a/b\n",
			check:  "[ -e /denied/a ] && echo escaped || echo contained\n",
		},
		{
			name:   "rm -rf under symlinked parent",
			script: "rm -rf /safe/out/keep\n",
			check:  "[ -e /denied/keep/victim ] && echo contained || echo escaped\n",
		},
		{
			name:   "mv out of symlinked parent",
			script: "mv /safe/out/keep/victim /safe/stolen\n",
			check:  "[ -e /denied/keep/victim ] && echo contained || echo escaped\n",
		},
	}

	for _, mode := range []policy.SymlinkMode{policy.SymlinkDeny, policy.SymlinkFollow} {
		for _, tt := range tests {
			t.Run(string(mode)+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				rt := newRuntime(t, &Config{
					FileSystem: CustomFileSystem(symlinkFSFactory{
						files: map[string]string{
							"/denied/keep/victim": "secret\n",
							"/safe/.keep":         "",
						},
						symlinks: map[string]string{
							"/safe/out":      "/denied",
							"/safe/dangling": "/denied/created",
						},
					}, defaultHomeDir),
					Policy: policy.NewStatic(&policy.Config{
						WriteRoots:  []string{"/safe"},
						SymlinkMode: mode,
					}),
				})
				session, err := rt.NewSession(context.Background())
				if err != nil {
					t.Fatalf("NewSession() error = %v", err)
				}

				result, err := session.Exec(context.Background(), &ExecutionRequest{Script: tt.script})
				if err != nil {
					t.Fatalf("Exec(script) error = %v", err)
				}
				if result.ExitCode == 0 {
					t.Fatalf("ExitCode = 0, want failure; stderr = %q", result.Stderr)
				}
				if !strings.Contains(result.Stderr, "denied") {
					t.Fatalf("Stderr = %q, want policy denial", result.Stderr)
				}

				result, err = session.Exec(context.Background(), &ExecutionRequest{Script: tt.check})
				if err != nil {
					t.Fatalf("Exec(check) error = %v", err)
				}
				if got, want := result.Stdout, "contained\n"; got != want {
					t.Fatalf("check Stdout = %q, want %q", got, want)
				}
			})
		}
	}
}
