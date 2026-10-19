package runtime

import (
	"context"
	"io"
	"os"
	"path"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
)

type seededFSFactory struct {
	files map[string]string
}

func (f seededFSFactory) New(ctx context.Context) (gbfs.FileSystem, error) {
	mem := gbfs.NewMemory()
	for name, contents := range f.files {
		if err := mem.MkdirAll(ctx, path.Dir(name), 0o755); err != nil {
			return nil, err
		}
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
