package runtime

import (
	"context"
	"io"
	"os"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
	"github.com/ewhauser/gbash/policy"
)

type seededFSFactory struct {
	files map[string]string
}

func (f seededFSFactory) New(ctx context.Context) (gbfs.FileSystem, error) {
	mem := gbfs.NewMemory()
	for name, contents := range f.files {
		file, err := mem.OpenFile(ctx, name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(file, contents); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	}
	return mem, nil
}

func TestOverlayFactorySupportsShellReadsAndCopyOnWrite(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{
		FileSystem: CustomFileSystem(
			gbfs.Overlay(seededFSFactory{files: map[string]string{
				"/seed.txt": "seed\n",
			}}),
			defaultHomeDir,
		),
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "cat /seed.txt\necho upper > /seed.txt\ncat /seed.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "seed\nupper\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestOverlayFollowsLowerSymlinksForShellReadsAndWrites(t *testing.T) {
	t.Parallel()
	lower := gbfs.FactoryFunc(func(ctx context.Context) (gbfs.FileSystem, error) {
		mem, err := seededFSFactory{files: map[string]string{
			"/w/real/f": "orig\n",
		}}.New(ctx)
		if err != nil {
			return nil, err
		}
		if err := mem.Symlink(ctx, "real", "/w/link"); err != nil {
			return nil, err
		}
		if err := mem.Symlink(ctx, "real/f", "/w/flink"); err != nil {
			return nil, err
		}
		return mem, nil
	})
	rt := newRuntime(t, &Config{
		FileSystem: CustomFileSystem(gbfs.Overlay(lower), defaultHomeDir),
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:   []string{"/"},
			WriteRoots:  []string{"/"},
			SymlinkMode: policy.SymlinkFollow,
		}),
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"cd /w\n" +
			"[ -d link ] && echo link-is-dir\n" +
			"[ -f flink ] && echo flink-is-file\n" +
			"[ -L flink ] && echo flink-is-link\n" +
			"mkdir -p link/sub && [ -d real/sub ] && echo mkdir-through-link\n" +
			"echo new > flink\n" +
			"cat real/f\n" +
			"[ -L flink ] && echo flink-still-link\n" +
			"echo created > link/sub/g\n" +
			"cat real/sub/g\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	want := "" +
		"link-is-dir\n" +
		"flink-is-file\n" +
		"flink-is-link\n" +
		"mkdir-through-link\n" +
		"new\n" +
		"flink-still-link\n" +
		"created\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}
