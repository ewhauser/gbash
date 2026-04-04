package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdfs "io/fs"
	"math"
	"strings"

	"github.com/ewhauser/gbash/commands"
	gbfs "github.com/ewhauser/gbash/fs"
	"github.com/ewhauser/gbash/internal/builtins"
	"github.com/ewhauser/gbash/internal/commandutil"
	"github.com/ewhauser/gbash/internal/shell/fileutil"
	"github.com/ewhauser/gbash/shell/syntax"
	"github.com/ewhauser/gbash/shell/syntax/typedjson"
	"github.com/ewhauser/gbash/shellvariant"
)

type astDumpSource struct {
	script     string
	sourceName string
	detectPath string
}

func runBashInvocationAST(ctx context.Context, cfg Config, parsed *builtins.BashInvocation, runtimeOpts *runtimeOptions, stdin io.Reader, stdout io.Writer) (int, error) {
	if parsed == nil {
		parsed = &builtins.BashInvocation{Name: cfg.Name, Source: builtins.BashSourceStdin}
	}

	source, exitCode, err := loadASTDumpSource(ctx, cfg, parsed, runtimeOpts, stdin)
	if err != nil {
		return exitCode, err
	}

	variant := astDumpShellVariant(parsed, source.script, source.detectPath, runtimeOpts != nil && runtimeOpts.detect)
	program, err := syntax.NewParser(syntax.Variant(variant.SyntaxLang())).Parse(strings.NewReader(source.script), source.sourceName)
	if err != nil {
		var parseErr syntax.ParseError
		if errors.As(err, &parseErr) {
			return 2, errors.New(formatVariantParseError(&parseErr, variant))
		}
		var langErr syntax.LangError
		if errors.As(err, &langErr) {
			return 2, errors.New(langErr.Error())
		}
		return 1, fmt.Errorf("parse script: %w", err)
	}

	if err := (typedjson.EncodeOptions{Indent: "  "}).Encode(writerOrDiscard(stdout), program); err != nil {
		return 1, fmt.Errorf("encode AST: %w", err)
	}
	return 0, nil
}

func loadASTDumpSource(ctx context.Context, cfg Config, parsed *builtins.BashInvocation, runtimeOpts *runtimeOptions, stdin io.Reader) (astDumpSource, int, error) {
	if parsed == nil {
		parsed = &builtins.BashInvocation{Name: cfg.Name, Source: builtins.BashSourceStdin}
	}

	switch parsed.Source {
	case builtins.BashSourceFile:
		return loadASTDumpFileSource(ctx, cfg, parsed, runtimeOpts)
	default:
		script, _, exitCode, err := loadBashInvocationScript(parsed, stdin)
		if err != nil {
			return astDumpSource{}, exitCode, err
		}
		if parsed.Source == builtins.BashSourceCommandString && strings.HasPrefix(script, "\n") {
			script = script[1:]
		}
		return astDumpSource{
			script:     script,
			sourceName: astDumpSourceName(parsed),
		}, 0, nil
	}
}

func loadASTDumpFileSource(ctx context.Context, cfg Config, parsed *builtins.BashInvocation, runtimeOpts *runtimeOptions) (astDumpSource, int, error) {
	if runtimeOpts == nil {
		runtimeOpts = &runtimeOptions{}
	}

	//nolint:contextcheck // gbash.New does not accept context; runtime use remains scoped to this ctx-bound CLI invocation.
	rt, err := newRuntime(cfg, runtimeOpts)
	if err != nil {
		return astDumpSource{}, 1, fmt.Errorf("init runtime: %w", err)
	}
	session, err := rt.NewSession(ctx)
	if err != nil {
		return astDumpSource{}, 1, fmt.Errorf("new session: %w", err)
	}
	if exitCode, err := prepareBashInvocationScriptPath(ctx, session, parsed, runtimeOpts); err != nil {
		return astDumpSource{}, exitCode, err
	}

	script, err := readASTDumpSessionScript(ctx, session.FileSystem(), session.Limits().MaxFileBytes, runtimeOpts.defaultWorkingDir(), parsed.ScriptPath)
	if err != nil {
		return astDumpSource{}, astDumpErrorExitCode(err), err
	}
	return astDumpSource{
		script:     script,
		sourceName: astDumpSourceName(parsed),
		detectPath: parsed.ScriptPath,
	}, 0, nil
}

func readASTDumpSessionScript(ctx context.Context, fsys gbfs.FileSystem, maxFileBytes int64, workDir, scriptPath string) (string, error) {
	if fsys == nil {
		return "", errors.New("session filesystem unavailable")
	}

	source, err := fsys.Open(ctx, gbfs.Resolve(workDir, scriptPath))
	if err != nil {
		return "", astDumpScriptLoadError(scriptPath, err)
	}
	defer func() { _ = source.Close() }()

	data, err := readASTDumpScriptData(ctx, source, scriptPath, maxFileBytes)
	if err != nil {
		return "", err
	}
	if err := builtins.ValidateShellScriptFileData(scriptPath, data); err != nil {
		return "", &commands.ExitError{Code: 126, Err: err}
	}
	return string(data), nil
}

func readASTDumpScriptData(ctx context.Context, source io.Reader, sourcePath string, maxFileBytes int64) ([]byte, error) {
	reader := commandutil.ReaderWithContext(ctx, source)
	if maxFileBytes <= 0 || maxFileBytes == math.MaxInt64 {
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("read script %s: %w", sourcePath, err)
		}
		return data, nil
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read script %s: %w", sourcePath, err)
	}
	if int64(len(data)) > maxFileBytes {
		return nil, fmt.Errorf("%s: %w", sourcePath, commands.Diagnosticf("input exceeds maximum file size of %d bytes", maxFileBytes))
	}
	return data, nil
}

func astDumpSourceName(parsed *builtins.BashInvocation) string {
	if parsed == nil {
		return "stdin"
	}
	switch {
	case strings.TrimSpace(parsed.ScriptPath) != "":
		return parsed.ScriptPath
	case strings.TrimSpace(parsed.ExecutionName) != "":
		return parsed.ExecutionName
	default:
		return "stdin"
	}
}

func astDumpShellVariant(parsed *builtins.BashInvocation, script, detectPath string, detect bool) shellvariant.ShellVariant {
	if detect {
		if variant := detectedShellVariant(script, detectPath); variant.Resolved() {
			return variant
		}
		return shellvariant.Bash
	}
	if parsed != nil {
		if variant := parsed.DefaultShellVariant(); variant.Resolved() {
			return variant
		}
	}
	return shellvariant.Bash
}

func detectedShellVariant(script, scriptPath string) shellvariant.ShellVariant {
	if variant := shellvariant.FromInterpreter(fileutil.Shebang([]byte(script))); variant.Resolved() {
		return variant
	}
	if scriptPath != "" {
		if variant := shellvariant.FromPath(scriptPath); variant.Resolved() {
			return variant
		}
	}
	return shellvariant.Bash
}

func formatVariantParseError(err *syntax.ParseError, variant shellvariant.ShellVariant) string {
	if err == nil {
		return ""
	}
	if variant.UsesBashDiagnostics() {
		return err.BashError()
	}
	return err.Error()
}

func astDumpErrorExitCode(err error) int {
	if err == nil {
		return 0
	}
	if code, ok := commands.ExitCode(err); ok {
		return code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 124
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}

func astDumpScriptLoadError(scriptPath string, err error) error {
	if astDumpScriptLoadIsNotExist(err) {
		return &commands.ExitError{
			Code: 127,
			Err:  fmt.Errorf("%s: No such file or directory", scriptPath),
		}
	}

	code := 1
	if exitCode, ok := commands.ExitCode(err); ok {
		code = exitCode
	}
	return &commands.ExitError{
		Code: code,
		Err:  fmt.Errorf("%s: %s", scriptPath, astDumpScriptLoadErrorText(err)),
	}
}

func astDumpScriptLoadErrorText(err error) string {
	switch {
	case astDumpScriptLoadIsNotExist(err):
		return "No such file or directory"
	case astDumpScriptLoadIsDirectory(err):
		return "Is a directory"
	default:
		return err.Error()
	}
}

func astDumpScriptLoadIsNotExist(err error) bool {
	return err != nil &&
		(errors.Is(err, stdfs.ErrNotExist) ||
			strings.Contains(strings.ToLower(err.Error()), "no such file or directory") ||
			strings.Contains(strings.ToLower(err.Error()), "file does not exist"))
}

func astDumpScriptLoadIsDirectory(err error) bool {
	if err == nil {
		return false
	}
	var pathErr *stdfs.PathError
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, stdfs.ErrInvalid) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "is a directory")
}
