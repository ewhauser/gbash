package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func findMake() (string, error) {
	for _, candidate := range []string{"gmake", "make"} {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("could not find make or gmake")
}

func requireTool(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("missing required tool %q", name)
	}
	return nil
}

func requireCC() error {
	for _, candidate := range []string{os.Getenv("CC"), "cc", "clang", "gcc"} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if _, err := exec.LookPath(candidate); err == nil {
			return nil
		}
	}
	return fmt.Errorf("missing required C compiler (tried $CC, cc, clang, gcc)")
}

func ensureSourceCache(ctx context.Context, mf *manifest, cacheDir string) (string, error) {
	downloadsDir := filepath.Join(cacheDir, "downloads")
	sourceRoot := filepath.Join(cacheDir, "src")
	if err := os.MkdirAll(downloadsDir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		return "", err
	}

	tarballPath := filepath.Join(downloadsDir, filepath.Base(mf.TarballURL))
	if _, err := os.Stat(tarballPath); errorsIsNotExist(err) {
		if err := downloadFile(ctx, mf.TarballURL, tarballPath); err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(mf.TarballSHA256) != "" {
		if err := verifySHA256(tarballPath, mf.TarballSHA256); err != nil {
			return "", err
		}
	}

	sourceDir := filepath.Join(sourceRoot, "coreutils-"+mf.GNUVersion)
	if _, err := os.Stat(sourceDir); err == nil {
		cacheCurrent, err := sourceCacheCurrent(sourceDir)
		if err != nil {
			return "", err
		}
		if cacheCurrent {
			return sourceDir, nil
		}
		if err := os.RemoveAll(sourceDir); err != nil {
			return "", err
		}
	}
	if err := extractTarGz(tarballPath, sourceRoot); err != nil {
		return "", err
	}
	if err := writeSourceCacheVersion(sourceDir); err != nil {
		return "", err
	}
	return sourceDir, nil
}

func findPreviousBuild(cacheDir, version string) (string, error) {
	workRoot := filepath.Join(cacheDir, "work")
	entries, err := os.ReadDir(workRoot)
	if err != nil {
		return "", err
	}
	prefix := "coreutils-" + version + "-"
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		candidate := filepath.Join(workRoot, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "config.status")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no previous build found for coreutils-%s", version)
}

func prepareWorkDir(cacheDir, version, sourceDir string) (string, error) {
	workRoot := filepath.Join(cacheDir, "work")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		return "", err
	}
	workDir, err := os.MkdirTemp(workRoot, "coreutils-"+version+"-")
	if err != nil {
		return "", err
	}
	if err := copyTree(sourceDir, workDir); err != nil {
		_ = os.RemoveAll(workDir)
		return "", fmt.Errorf("copy source tree: %w", err)
	}
	return workDir, nil
}

func prepareWorkDirFromPreparedArchive(ctx context.Context, cacheDir, version, archivePath string) (string, error) {
	workRoot := filepath.Join(cacheDir, "work")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		return "", err
	}
	workDir, err := os.MkdirTemp(workRoot, "coreutils-"+version+"-")
	if err != nil {
		return "", err
	}
	if err := extractTarGz(archivePath, workDir); err != nil {
		_ = os.RemoveAll(workDir)
		return "", fmt.Errorf("extract prepared GNU build archive: %w", err)
	}
	if err := relocatePreparedBuild(ctx, workDir); err != nil {
		_ = os.RemoveAll(workDir)
		return "", err
	}
	return workDir, nil
}

func buildPreparedBuildArchive(ctx context.Context, makeBin, cacheDir, version, sourceDir, archivePath string, keepWorkdir bool) error {
	workDir, err := prepareWorkDir(cacheDir, version, sourceDir)
	if err != nil {
		return err
	}
	if !keepWorkdir {
		defer func() { _ = os.RemoveAll(workDir) }()
	}
	if err := configureAndBuild(ctx, makeBin, workDir); err != nil {
		return err
	}
	if err := archiveDirectoryAsTarGz(workDir, archivePath); err != nil {
		return err
	}
	return nil
}

func relocatePreparedBuild(_ context.Context, workDir string) error {
	originalWorkDir, err := preparedBuildOriginalWorkDir(workDir)
	if err != nil {
		return fmt.Errorf("relocate prepared GNU build: %w", err)
	}
	if originalWorkDir == "" || originalWorkDir == workDir {
		return nil
	}

	return filepath.Walk(workDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.IndexByte(data, 0) >= 0 || !bytes.Contains(data, []byte(originalWorkDir)) {
			return nil
		}

		updated := bytes.ReplaceAll(data, []byte(originalWorkDir), []byte(workDir))
		if err := os.WriteFile(path, updated, info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chtimes(path, info.ModTime(), info.ModTime())
	})
}

func preparedBuildOriginalWorkDir(workDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(workDir, "config.status"))
	if err != nil {
		return "", err
	}
	const prefix = "ac_pwd='"
	start := bytes.Index(data, []byte(prefix))
	if start == -1 {
		return "", nil
	}
	start += len(prefix)
	end := bytes.IndexByte(data[start:], '\'')
	if end == -1 {
		return "", fmt.Errorf("could not parse original workdir from config.status")
	}
	return string(data[start : start+end]), nil
}

func archiveDirectoryAsTarGz(sourceDir, archivePath string) error {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return err
	}
	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzw := gzip.NewWriter(file)
	defer func() { _ = gzw.Close() }()

	tw := tar.NewWriter(gzw)
	defer func() { _ = tw.Close() }()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourceDir {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		linkTarget := ""
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return err
		}
		hdr.Name = rel
		if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = src.Close() }()
		if _, err := io.Copy(tw, src); err != nil {
			return err
		}
		return nil
	})
}

func configureAndBuild(ctx context.Context, makeBin, workDir string) error {
	configure := exec.CommandContext(ctx, "./configure", "--disable-nls", "--disable-dependency-tracking")
	configure.Dir = workDir
	configure.Stdout = os.Stdout
	configure.Stderr = os.Stderr
	if err := configure.Run(); err != nil {
		return fmt.Errorf("configure GNU coreutils: %w", err)
	}

	makeCmd := exec.CommandContext(ctx, makeBin, "-j", fmt.Sprintf("%d", maxInt(runtime.NumCPU(), 2)))
	makeCmd.Dir = workDir
	makeCmd.Stdout = os.Stdout
	makeCmd.Stderr = os.Stderr
	if err := makeCmd.Run(); err != nil {
		return fmt.Errorf("build GNU coreutils: %w", err)
	}
	return nil
}

func copyTree(sourceDir, destination string) error {
	type dirModTime struct {
		path    string
		modTime time.Time
	}
	var dirs []dirModTime
	if err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourceDir {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		case info.IsDir():
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			dirs = append(dirs, dirModTime{path: target, modTime: info.ModTime()})
			return nil
		case info.Mode().IsRegular():
			if err := copyFile(path, target, info); err != nil {
				return err
			}
			return nil
		default:
			return fmt.Errorf("copy source tree: unsupported file mode %s for %s", info.Mode(), path)
		}
	}); err != nil {
		return err
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chtimes(dirs[i].path, dirs[i].modTime, dirs[i].modTime); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(sourcePath, targetPath string, info os.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Chtimes(targetPath, info.ModTime(), info.ModTime())
}

func downloadFile(ctx context.Context, url, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}
	tmpPath := destination + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, destination)
}

func verifySHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(sum, expected) {
		return fmt.Errorf("sha256 mismatch for %s: got %s want %s", path, sum, expected)
	}
	return nil
}

func extractTarGz(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()
	tr := tar.NewReader(gzr)
	type dirModTime struct {
		path    string
		modTime time.Time
	}
	var dirs []dirModTime
	for {
		header, err := tr.Next()
		if err == io.EOF {
			for i := len(dirs) - 1; i >= 0; i-- {
				if err := os.Chtimes(dirs[i].path, dirs[i].modTime, dirs[i].modTime); err != nil {
					return err
				}
			}
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destination, header.Name)
		modTime := header.ModTime
		if modTime.IsZero() {
			modTime = time.Unix(0, 0)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, header.FileInfo().Mode()); err != nil {
				return err
			}
			dirs = append(dirs, dirModTime{path: target, modTime: modTime})
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, header.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			if err := os.Chtimes(target, modTime, modTime); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil && !os.IsExist(err) {
				return err
			}
		}
	}
}

func sourceCacheCurrent(sourceDir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(sourceDir, ".gbash-cache-version"))
	if errorsIsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(data)) == sourceCacheVersion, nil
}

func writeSourceCacheVersion(sourceDir string) error {
	return os.WriteFile(filepath.Join(sourceDir, ".gbash-cache-version"), []byte(sourceCacheVersion+"\n"), 0o644)
}

func ensureExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, want gbash binary", path)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", path)
	}
	return nil
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
