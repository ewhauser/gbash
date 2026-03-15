package sqlitefs

import (
	"context"
	"fmt"
	"os"
	"path"

	gbfs "github.com/ewhauser/gbash/fs"
)

// SeedTemplate initializes dbPath with the provided initial files so benchmark
// runs can copy a single prepared SQLite database outside the timed region.
func SeedTemplate(ctx context.Context, dbPath string, files gbfs.InitialFiles) error {
	fsys, err := newSQLiteFS(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = fsys.close() }()

	for name, initial := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		parent := path.Dir(name)
		if err := fsys.MkdirAll(ctx, parent, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", parent, err)
		}

		contents := initial.Content
		if initial.Lazy != nil {
			loaded, err := initial.Lazy(ctx)
			if err != nil {
				return fmt.Errorf("load %s: %w", name, err)
			}
			contents = loaded
		}

		mode := initial.Mode.Perm()
		if mode == 0 {
			mode = 0o644
		}
		file, err := fsys.OpenFile(ctx, name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return fmt.Errorf("open %s: %w", name, err)
		}
		if len(contents) > 0 {
			if _, err := file.Write(contents); err != nil {
				_ = file.Close()
				return fmt.Errorf("write %s: %w", name, err)
			}
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close %s: %w", name, err)
		}
		if initial.Mode != 0 {
			if err := fsys.Chmod(ctx, name, initial.Mode.Perm()); err != nil {
				return fmt.Errorf("chmod %s: %w", name, err)
			}
		}
		if !initial.ModTime.IsZero() {
			if err := fsys.Chtimes(ctx, name, initial.ModTime, initial.ModTime); err != nil {
				return fmt.Errorf("chtimes %s: %w", name, err)
			}
		}
	}
	return nil
}
