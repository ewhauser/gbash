package builtins

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	stdfs "io/fs"
	"math"
	"os"
	"path"
	"sort"
	"strings"

	gbfs "github.com/ewhauser/gbash/fs"
)

type Tar struct{}

type tarMode string

const (
	tarModeCreate  tarMode = "create"
	tarModeExtract tarMode = "extract"
	tarModeList    tarMode = "list"
)

type tarOptions struct {
	mode     tarMode
	archive  string
	chdir    string
	gzip     bool
	verbose  bool
	toStdout bool
	keepOld  bool
}

func NewTar() *Tar {
	return &Tar{}
}

func (c *Tar) Name() string {
	return "tar"
}

func (c *Tar) Run(ctx context.Context, inv *Invocation) error {
	return RunCommand(ctx, c, inv)
}

func (c *Tar) Spec() CommandSpec {
	return CommandSpec{
		Name:  "tar",
		About: "Archive helper inside the gbash sandbox.",
		Usage: "tar -c[f] ARCHIVE [PATH...]\n  tar -x[f] ARCHIVE\n  tar -t[f] ARCHIVE",
		Options: []OptionSpec{
			{Name: "create", Short: 'c', Long: "create", Help: "create archive"},
			{Name: "extract", Short: 'x', Long: "extract", Help: "extract archive"},
			{Name: "list", Short: 't', Long: "list", Help: "list archive contents"},
			{Name: "file", Short: 'f', Long: "file", Arity: OptionRequiredValue, ValueName: "ARCHIVE", Help: "read/write ARCHIVE instead of stdin/stdout"},
			{Name: "directory", Short: 'C', Arity: OptionRequiredValue, ValueName: "DIR", Help: "use DIR as create base or extract destination"},
			{Name: "gzip", Short: 'z', Long: "gzip", Help: "gzip-compress or gzip-decompress the archive stream"},
			{Name: "verbose", Short: 'v', Long: "verbose", Help: "verbose entry listing to stderr"},
			{Name: "to-stdout", Short: 'O', Long: "to-stdout", Help: "write extracted file contents to stdout"},
			{Name: "keep-old", Short: 'k', Long: "keep-old-files", Help: "keep existing files on extract"},
		},
		Args: []ArgSpec{
			{Name: "path", ValueName: "PATH", Repeatable: true},
		},
		Parse: ParseConfig{
			InferLongOptions:         true,
			GroupShortOptions:        true,
			ShortOptionValueAttached: true,
			LongOptionValueEquals:    true,
			AutoHelp:                 true,
		},
		HelpRenderer: renderStaticHelp(tarHelpText),
	}
}

func (c *Tar) RunParsed(ctx context.Context, inv *Invocation, matches *ParsedCommand) error {
	opts, err := parseTarMatches(inv, matches)
	if err != nil {
		return err
	}
	operands := matches.Args("path")

	switch opts.mode {
	case tarModeCreate:
		if len(operands) == 0 {
			return exitf(inv, 1, "tar: Cowardly refusing to create an empty archive")
		}
		return runTarCreate(ctx, inv, &opts, operands)
	case tarModeExtract:
		return runTarExtract(ctx, inv, &opts)
	case tarModeList:
		return runTarList(ctx, inv, &opts)
	default:
		return exitf(inv, 1, "tar: you must specify one of -c, -x, or -t")
	}
}

func parseTarMatches(inv *Invocation, matches *ParsedCommand) (tarOptions, error) {
	opts := tarOptions{
		archive: "-",
	}
	if matches.Has("create") {
		if err := tarSetMode(&opts, tarModeCreate, inv); err != nil {
			return tarOptions{}, err
		}
	}
	if matches.Has("extract") {
		if err := tarSetMode(&opts, tarModeExtract, inv); err != nil {
			return tarOptions{}, err
		}
	}
	if matches.Has("list") {
		if err := tarSetMode(&opts, tarModeList, inv); err != nil {
			return tarOptions{}, err
		}
	}
	if opts.mode == "" {
		return tarOptions{}, exitf(inv, 1, "tar: you must specify one of -c, -x, or -t")
	}
	if matches.Has("file") {
		opts.archive = matches.Value("file")
	}
	if matches.Has("directory") {
		opts.chdir = matches.Value("directory")
	}
	opts.gzip = matches.Has("gzip")
	opts.verbose = matches.Has("verbose")
	opts.toStdout = matches.Has("to-stdout")
	opts.keepOld = matches.Has("keep-old")
	if opts.toStdout && opts.mode != tarModeExtract {
		return tarOptions{}, exitf(inv, 1, "tar: -O is only supported with -x")
	}
	return opts, nil
}

func tarSetMode(opts *tarOptions, mode tarMode, inv *Invocation) error {
	if opts.mode != "" && opts.mode != mode {
		return exitf(inv, 1, "tar: options -c, -x, and -t are mutually exclusive")
	}
	opts.mode = mode
	return nil
}

func runTarCreate(ctx context.Context, inv *Invocation, opts *tarOptions, operands []string) error {
	baseDir, err := tarBaseDir(ctx, inv, opts)
	if err != nil {
		return err
	}
	tw, closeArchive, err := openTarWriter(ctx, inv, opts)
	if err != nil {
		return err
	}
	var archive tarArchiveRef
	if opts.archive != "-" {
		archive.abs = allowPath(inv, opts.archive)
		if info, _, err := statPath(ctx, inv, archive.abs); err == nil {
			archive.id, archive.hasID = fileInfoIdentity(info)
		}
	}

	for _, operand := range operands {
		sourceAbs := gbfs.Resolve(baseDir, operand)
		info, _, err := lstatPath(ctx, inv, sourceAbs)
		if err != nil {
			return err
		}
		entryName := tarArchiveName(operand, sourceAbs)
		if entryName == "" {
			entryName = path.Base(sourceAbs)
		}
		if err := tarWritePath(ctx, inv, tw, archive, sourceAbs, entryName, info); err != nil {
			_ = closeArchive()
			return err
		}
	}
	return closeArchive()
}

func runTarList(ctx context.Context, inv *Invocation, opts *tarOptions) error {
	tr, closeArchive, err := openTarReader(ctx, inv, opts)
	if err != nil {
		return err
	}
	defer func() { _ = closeArchive() }()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return exitf(inv, 1, "tar: %v", err)
		}
		name, err := sanitizeTarEntryName(header.Name)
		if err != nil {
			return exitf(inv, 1, "tar: %v", err)
		}
		if name == "" {
			continue
		}
		if _, err := fmt.Fprintln(inv.Stdout, name); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
	}
}

func runTarExtract(ctx context.Context, inv *Invocation, opts *tarOptions) error {
	baseDir, err := tarBaseDir(ctx, inv, opts)
	if err != nil {
		return err
	}
	tr, closeArchive, err := openTarReader(ctx, inv, opts)
	if err != nil {
		return err
	}
	defer func() { _ = closeArchive() }()

	budget := newTarExtractBudget(inv)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return exitf(inv, 1, "tar: %v", err)
		}
		if err := budget.charge(header); err != nil {
			return exitf(inv, 1, "tar: %s: %v", header.Name, err)
		}
		entryName, err := sanitizeTarEntryName(header.Name)
		if err != nil {
			return exitf(inv, 1, "tar: %v", err)
		}
		if entryName == "" {
			continue
		}

		if opts.verbose {
			_, _ = fmt.Fprintln(inv.Stderr, entryName)
		}
		if opts.toStdout {
			if tarIsRegularType(header.Typeflag) {
				if err := copyBounded(ctx, inv.Stdout, tr, fileSizeLimit(inv)); err != nil {
					return &ExitError{Code: 1, Err: err}
				}
			}
			continue
		}

		targetAbs := gbfs.Resolve(baseDir, entryName)
		if err := tarCheckContained(ctx, inv, baseDir, entryName, targetAbs, header.Typeflag == tar.TypeDir); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := tarEnsureDir(ctx, inv, targetAbs, stdfs.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, 0:
			if err := tarExtractFile(ctx, inv, tr, targetAbs, header, opts.keepOld); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := tarExtractSymlink(ctx, inv, baseDir, entryName, targetAbs, header, opts.keepOld); err != nil {
				return err
			}
		case tar.TypeLink:
			if err := tarExtractHardlink(ctx, inv, baseDir, targetAbs, header, opts.keepOld); err != nil {
				return err
			}
		default:
			return exitf(inv, 1, "tar: unsupported entry type %q", string(header.Typeflag))
		}
	}
}

// tarExtractMultiplier scales MaxFileBytes into the cap on the total bytes
// one tar -x may write, so a small compressed archive of many full-size
// entries cannot expand without bound.
const tarExtractMultiplier = 16

// tarMaxExtractEntries caps how many members one tar -x will process.
const tarMaxExtractEntries = 100_000

var errTarExtractBudget = errors.New("archive exceeds extraction limit")

// tarExtractBudget tracks the cumulative size and member count of an
// extraction against its limits.
type tarExtractBudget struct {
	maxBytes int64
	bytes    int64
	entries  int
}

func newTarExtractBudget(inv *Invocation) *tarExtractBudget {
	b := &tarExtractBudget{}
	if limit := fileSizeLimit(inv); limit > 0 {
		b.maxBytes = math.MaxInt64
		if limit <= math.MaxInt64/tarExtractMultiplier {
			b.maxBytes = limit * tarExtractMultiplier
		}
	}
	return b
}

func (b *tarExtractBudget) charge(header *tar.Header) error {
	b.entries++
	if b.entries > tarMaxExtractEntries {
		return errTarExtractBudget
	}
	if b.maxBytes <= 0 || !tarIsRegularType(header.Typeflag) || header.Size <= 0 {
		return nil
	}
	if header.Size > b.maxBytes-b.bytes {
		return errTarExtractBudget
	}
	b.bytes += header.Size
	return nil
}

func tarBaseDir(ctx context.Context, inv *Invocation, opts *tarOptions) (string, error) {
	if opts.chdir == "" {
		return inv.Cwd, nil
	}
	info, abs, err := statPath(ctx, inv, opts.chdir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", exitf(inv, 1, "tar: %s: Not a directory", opts.chdir)
	}
	return abs, nil
}

func openTarWriter(ctx context.Context, inv *Invocation, opts *tarOptions) (*tar.Writer, func() error, error) {
	writer := inv.Stdout
	closers := make([]io.Closer, 0, 3)

	if opts.archive != "-" {
		archiveAbs := allowPath(inv, opts.archive)
		if err := ensureParentDirExists(ctx, inv, archiveAbs); err != nil {
			return nil, nil, err
		}
		file, err := inv.FS.OpenFile(ctx, archiveAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, nil, &ExitError{Code: 1, Err: err}
		}
		writer = file
		closers = append(closers, file)
	}

	if opts.gzip {
		zw := gzip.NewWriter(writer)
		writer = zw
		closers = append([]io.Closer{zw}, closers...)
	}

	tw := tar.NewWriter(writer)
	closers = append([]io.Closer{tw}, closers...)

	return tw, func() error {
		var first error
		for _, closer := range closers {
			if err := closer.Close(); err != nil && first == nil {
				first = err
			}
		}
		return first
	}, nil
}

func openTarReader(ctx context.Context, inv *Invocation, opts *tarOptions) (*tar.Reader, func() error, error) {
	reader := inv.Stdin
	closers := make([]io.Closer, 0, 2)

	if opts.archive != "-" {
		file, _, err := openRead(ctx, inv, opts.archive)
		if err != nil {
			return nil, nil, err
		}
		reader = file
		closers = append(closers, file)
	}

	if opts.gzip {
		zr, err := gzip.NewReader(reader)
		if err != nil {
			for _, closer := range closers {
				_ = closer.Close()
			}
			return nil, nil, exitf(inv, 1, "tar: %v", err)
		}
		reader = zr
		closers = append([]io.Closer{zr}, closers...)
	}

	return tar.NewReader(reader), func() error {
		var first error
		for _, closer := range closers {
			if err := closer.Close(); err != nil && first == nil {
				first = err
			}
		}
		return first
	}, nil
}

// tarArchiveRef identifies the archive being written so tar -c can skip it
// whether it is reached by its own path or through a hard link.
type tarArchiveRef struct {
	abs   string
	id    fileInfoIdentityKey
	hasID bool
}

func (a tarArchiveRef) matches(abs string, info stdfs.FileInfo) bool {
	if a.abs == "" {
		return false
	}
	if abs == a.abs {
		return true
	}
	if !a.hasID || !info.Mode().IsRegular() {
		return false
	}
	id, ok := fileInfoIdentity(info)
	return ok && id == a.id
}

func tarWritePath(ctx context.Context, inv *Invocation, tw *tar.Writer, archive tarArchiveRef, abs, name string, info stdfs.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if archive.matches(abs, info) {
		_, _ = fmt.Fprintf(inv.Stderr, "tar: %s: file is the archive; not dumped\n", name)
		return nil
	}
	if err := tarWriteHeader(ctx, inv, tw, abs, name, info); err != nil {
		return err
	}
	if info.Mode()&stdfs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		if info.IsDir() {
			return tarWriteDirChildren(ctx, inv, tw, archive, abs, name)
		}
		return nil
	}

	file, _, err := openRead(ctx, inv, abs)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if err := copyBounded(ctx, tw, file, 0); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

func tarWriteHeader(ctx context.Context, inv *Invocation, tw *tar.Writer, abs, name string, info stdfs.FileInfo) error {
	headerName := path.Clean(name)
	if headerName != "." && strings.HasPrefix(name, "./") {
		headerName = "./" + headerName
	}
	if headerName == "" || headerName == "/" {
		return nil
	}

	var (
		header *tar.Header
		err    error
	)
	switch {
	case info.Mode()&stdfs.ModeSymlink != 0:
		target, readErr := inv.FS.Readlink(ctx, abs)
		if readErr != nil {
			return &ExitError{Code: 1, Err: readErr}
		}
		header = &tar.Header{
			Name:     headerName,
			Typeflag: tar.TypeSymlink,
			Linkname: target,
			Mode:     int64(info.Mode().Perm()),
			ModTime:  info.ModTime(),
		}
	case info.IsDir():
		header, err = tar.FileInfoHeader(info, "")
		if err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		header.Name = strings.TrimSuffix(headerName, "/") + "/"
	case info.Mode().IsRegular():
		header, err = tar.FileInfoHeader(info, "")
		if err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		header.Name = headerName
	default:
		return exitf(inv, 1, "tar: unsupported file type for %s", abs)
	}
	if err := tw.WriteHeader(header); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

func tarWriteDirChildren(ctx context.Context, inv *Invocation, tw *tar.Writer, archive tarArchiveRef, abs, name string) error {
	entries, err := readDir(ctx, inv, abs)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		childAbs := path.Join(abs, entry.Name())
		childInfo, _, err := lstatPath(ctx, inv, childAbs)
		if err != nil {
			return err
		}
		if err := tarWritePath(ctx, inv, tw, archive, childAbs, tarChildName(name, entry.Name()), childInfo); err != nil {
			return err
		}
	}
	return nil
}

func tarArchiveName(arg, sourceAbs string) string {
	if strings.HasPrefix(arg, "/") {
		return strings.TrimPrefix(path.Clean(arg), "/")
	}
	name := path.Clean(arg)
	if name == "." {
		// GNU tar names the members of "." as "./" and "./child".
		return "."
	}
	if name == "/" {
		return path.Base(sourceAbs)
	}
	return strings.TrimPrefix(name, "./")
}

// tarChildName joins a directory member name with a child, keeping the
// leading "./" that GNU tar uses for members archived from ".".
func tarChildName(dir, child string) string {
	if dir == "." || strings.HasPrefix(dir, "./") {
		return "./" + path.Join(dir, child)
	}
	return path.Join(dir, child)
}

func tarIsRegularType(typeflag byte) bool {
	return typeflag == tar.TypeReg || typeflag == 0
}

func sanitizeTarEntryName(name string) (string, error) {
	if name == "" {
		return "", nil
	}
	cleaned := path.Clean(strings.TrimLeft(name, "/"))
	switch {
	case cleaned == ".", cleaned == "/":
		return "", nil
	case cleaned == "..", strings.HasPrefix(cleaned, "../"):
		return "", fmt.Errorf("unsafe archive path %q", name)
	default:
		return cleaned, nil
	}
}

func tarEnsureDir(ctx context.Context, inv *Invocation, targetAbs string, perm stdfs.FileMode) error {
	if err := inv.FS.MkdirAll(ctx, targetAbs, perm.Perm()); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

func tarExtractFile(ctx context.Context, inv *Invocation, tr *tar.Reader, targetAbs string, header *tar.Header, keepOld bool) error {
	if header.Size < 0 || exceedsFileSizeLimit(inv, uint64(header.Size)) {
		return exitf(inv, 1, "tar: %s: Cannot write: %v", targetAbs, errFileTooLarge)
	}
	if err := tarEnsureParents(ctx, inv, targetAbs); err != nil {
		return err
	}
	if exists, err := gzipTargetExists(ctx, inv, targetAbs); err != nil {
		return err
	} else if exists {
		if keepOld {
			return exitf(inv, 1, "tar: %s: Cannot open: File exists", targetAbs)
		}
		if err := inv.FS.Remove(ctx, targetAbs, false); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
	}

	file, err := inv.FS.OpenFile(ctx, targetAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, stdfs.FileMode(header.Mode).Perm())
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if err := copyBounded(ctx, file, tr, fileSizeLimit(inv)); err != nil {
		_ = file.Close()
		_ = inv.FS.Remove(ctx, targetAbs, false)
		if errors.Is(err, errFileTooLarge) {
			return exitf(inv, 1, "tar: %s: Cannot write: %v", targetAbs, err)
		}
		return &ExitError{Code: 1, Err: err}
	}
	if err := file.Close(); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if !header.ModTime.IsZero() {
		mtime := header.ModTime
		if err := inv.FS.Chtimes(ctx, targetAbs, mtime, mtime); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
	}
	recordFileMutation(inv.TraceRecorder(), "write", targetAbs, "", targetAbs)
	return nil
}

func tarExtractSymlink(ctx context.Context, inv *Invocation, baseDir, entryName, targetAbs string, header *tar.Header, keepOld bool) error {
	if err := tarValidateSymlinkTarget(entryName, header.Linkname); err != nil {
		return exitf(inv, 1, "tar: %v", err)
	}
	if err := tarEnsureParents(ctx, inv, targetAbs); err != nil {
		return err
	}
	if err := tarCheckSymlinkResolved(ctx, inv, baseDir, targetAbs, header.Linkname); err != nil {
		return err
	}
	if exists, err := gzipTargetExists(ctx, inv, targetAbs); err != nil {
		return err
	} else if exists {
		if keepOld {
			return exitf(inv, 1, "tar: %s: Cannot create symlink: File exists", targetAbs)
		}
		// Like GNU tar, only replace files and empty directories; never
		// recursively delete existing data to make room for a symlink.
		if err := inv.FS.Remove(ctx, targetAbs, false); err != nil {
			return exitf(inv, 1, "tar: %s: Cannot create symlink: File exists", targetAbs)
		}
	}
	if err := inv.FS.Symlink(ctx, header.Linkname, targetAbs); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

func tarExtractHardlink(ctx context.Context, inv *Invocation, baseDir, targetAbs string, header *tar.Header, keepOld bool) error {
	linkName, err := sanitizeTarEntryName(header.Linkname)
	if err != nil {
		return exitf(inv, 1, "tar: %v", err)
	}
	linkAbs := gbfs.Resolve(baseDir, linkName)
	if err := tarCheckContained(ctx, inv, baseDir, linkName, linkAbs, true); err != nil {
		return err
	}
	if err := tarEnsureParents(ctx, inv, targetAbs); err != nil {
		return err
	}
	if exists, err := gzipTargetExists(ctx, inv, targetAbs); err != nil {
		return err
	} else if exists {
		if keepOld {
			return exitf(inv, 1, "tar: %s: Cannot create link: File exists", targetAbs)
		}
		if err := inv.FS.Remove(ctx, targetAbs, false); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
	}
	if err := inv.FS.Link(ctx, linkAbs, targetAbs); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

func tarValidateSymlinkTarget(entryName, target string) error {
	if target == "" {
		return nil
	}
	if strings.HasPrefix(target, "/") {
		return fmt.Errorf("unsafe symlink target %q", target)
	}
	resolved := path.Clean(path.Join(path.Dir(entryName), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("unsafe symlink target %q", target)
	}
	return nil
}

// tarCheckSymlinkResolved resolves a relative symlink target against the
// real path of the link's parent, which may differ from the lexical entry
// path once earlier members have created symlinked directories, and rejects
// targets that leave the real extraction root.
func tarCheckSymlinkResolved(ctx context.Context, inv *Invocation, baseDir, targetAbs, linkTarget string) error {
	if linkTarget == "" {
		return nil
	}
	realBase, err := inv.FS.Realpath(ctx, baseDir)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	realParent, err := inv.FS.Realpath(ctx, path.Dir(targetAbs))
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if !tarPathWithin(path.Join(realParent, linkTarget), realBase) {
		return exitf(inv, 1, "tar: unsafe symlink target %q", linkTarget)
	}
	return nil
}

// tarCheckContained rejects an extraction path whose deepest existing
// ancestor (or the path itself when includeTarget is set) resolves outside
// baseDir, so symlinks created earlier in the archive cannot redirect later
// writes or hardlink sources out of the extraction root.
func tarCheckContained(ctx context.Context, inv *Invocation, baseDir, entryName, targetAbs string, includeTarget bool) error {
	realBase, err := inv.FS.Realpath(ctx, baseDir)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	current := targetAbs
	if !includeTarget {
		current = path.Dir(targetAbs)
	}
	for {
		if !tarPathWithin(current, baseDir) {
			return nil
		}
		_, _, exists, err := lstatMaybe(ctx, inv, current)
		if err != nil {
			return &ExitError{Code: exitCodeForError(err), Err: err}
		}
		if exists {
			resolved, err := inv.FS.Realpath(ctx, current)
			if err != nil || !tarPathWithin(resolved, realBase) {
				return exitf(inv, 1, "tar: %s: Cannot open: path escapes extraction directory through a symlink", entryName)
			}
			return nil
		}
		if current == baseDir {
			return nil
		}
		current = path.Dir(current)
	}
}

func tarPathWithin(name, root string) bool {
	return root == "/" || name == root || strings.HasPrefix(name, root+"/")
}

func tarEnsureParents(ctx context.Context, inv *Invocation, targetAbs string) error {
	parent := path.Dir(targetAbs)
	if parent == "." || parent == "/" {
		return nil
	}
	if err := inv.FS.MkdirAll(ctx, parent, 0o755); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

const tarHelpText = `tar - archive helper inside the gbash sandbox

Usage:
  tar -c[f] ARCHIVE [PATH...]
  tar -x[f] ARCHIVE
  tar -t[f] ARCHIVE

Supported options:
  -c        create archive
  -x        extract archive
  -t        list archive contents
  -f FILE   read/write FILE instead of stdin/stdout
  -C DIR    use DIR as create base or extract destination
  -z        gzip-compress or gzip-decompress the archive stream
  -v        verbose entry listing to stderr
  -O        write extracted file contents to stdout
  -k        keep existing files on extract
  --help    show this help

Notes:
  - leading slashes are stripped on extract
  - parent-traversal paths are rejected on extract
  - symlink targets that escape the extraction root are rejected
  - bzip2/xz/zstd and append/update modes are not implemented
`

var _ Command = (*Tar)(nil)
var _ SpecProvider = (*Tar)(nil)
var _ ParsedRunner = (*Tar)(nil)
