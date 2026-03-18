package interpreter

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ewhauser/gbash/commands"
	"github.com/ewhauser/gbash/internal/shfork/expand"
	"github.com/ewhauser/gbash/internal/shfork/interp"
)

type ResolvedCommand struct {
	Name    string
	Path    string
	Source  string
	Payload any
}

type CommandTrace struct {
	Dir              string
	Position         string
	ResolvedName     string
	ResolvedPath     string
	ResolutionSource string
	ExitCode         int
	Duration         time.Duration
}

type Deps struct {
	Resolve func(context.Context, string, expand.Environ, string) (*ResolvedCommand, bool, error)
	Allow   func(context.Context, string, []string) error
	Invoke  func(context.Context, *ResolvedCommand, []string, map[string]string, string, io.Reader, io.Writer, io.Writer) (map[string]string, error)

	IsPolicyDenied func(error) bool
	PolicyFailure  func(context.Context, io.Writer, string, ...any) error
	NotFound       func(context.Context, io.Writer, string) error
	RecordDenied   func(error, string, string, string, string)

	RecordStart func([]string, CommandTrace)
	RecordExit  func([]string, CommandTrace)
}

type Facade struct {
	deps Deps
}

func New(deps Deps) *Facade {
	return &Facade{deps: deps}
}

func (f *Facade) NewRunner(config *interp.VirtualConfig, options ...interp.RunnerOption) (*interp.Runner, error) {
	return interp.NewVirtual(config, options...)
}

type ExecuteRequest struct {
	Argv          []string
	VirtualWD     string
	Env           expand.Environ
	CurrentEnv    map[string]string
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	Position      string
	Internal      bool
	FromBootstrap bool
	PrepareInvoke func(context.Context) context.Context
	SyncEnv       func(context.Context, map[string]string, map[string]string) error
}

func (f *Facade) Execute(ctx context.Context, req *ExecuteRequest) (map[string]string, error) {
	if req == nil {
		req = &ExecuteRequest{}
	}
	if len(req.Argv) == 0 {
		return req.CurrentEnv, nil
	}
	resolved, ok, err := f.deps.Resolve(ctx, req.VirtualWD, req.Env, req.Argv[0])
	if err != nil {
		if f.deps.IsPolicyDenied != nil && f.deps.IsPolicyDenied(err) {
			if f.deps.RecordDenied != nil {
				f.deps.RecordDenied(err, "stat", "", req.Argv[0], "")
			}
			if f.deps.PolicyFailure != nil {
				return req.CurrentEnv, f.deps.PolicyFailure(ctx, req.Stderr, "%v", err)
			}
		}
		return req.CurrentEnv, err
	}
	if !ok {
		if f.deps.NotFound != nil {
			return req.CurrentEnv, f.deps.NotFound(ctx, req.Stderr, req.Argv[0])
		}
		return req.CurrentEnv, fmt.Errorf("%s: command not found", req.Argv[0])
	}

	start := time.Now().UTC()
	if !req.Internal {
		if f.deps.Allow != nil {
			if err := f.deps.Allow(ctx, resolved.Name, req.Argv); err != nil {
				if f.deps.RecordDenied != nil {
					f.deps.RecordDenied(err, "", resolved.Path, resolved.Name, resolved.Source)
				}
				if f.deps.PolicyFailure != nil {
					return req.CurrentEnv, f.deps.PolicyFailure(ctx, req.Stderr, "%v", err)
				}
				return req.CurrentEnv, err
			}
		}
		if !req.FromBootstrap && f.deps.RecordStart != nil {
			f.deps.RecordStart(req.Argv, CommandTrace{
				Dir:              req.VirtualWD,
				Position:         req.Position,
				ResolvedName:     resolved.Name,
				ResolvedPath:     resolved.Path,
				ResolutionSource: resolved.Source,
			})
		}
	}

	invokeCtx := ctx
	if req.PrepareInvoke != nil {
		invokeCtx = req.PrepareInvoke(ctx)
	}
	finalEnv, err := f.deps.Invoke(invokeCtx, resolved, req.Argv, req.CurrentEnv, req.VirtualWD, req.Stdin, req.Stdout, req.Stderr)
	if req.SyncEnv != nil {
		if syncErr := req.SyncEnv(ctx, req.CurrentEnv, finalEnv); syncErr != nil {
			return finalEnv, syncErr
		}
	}

	exitCode := 0
	if err != nil {
		exitCode = 1
		if code, ok := commands.ExitCode(err); ok {
			exitCode = code
			if msg, ok := commands.DiagnosticMessage(err); ok && req.Stderr != nil {
				_, _ = fmt.Fprintln(req.Stderr, msg)
			}
			err = interp.ExitStatus(code)
		}
	}

	if !req.Internal && !req.FromBootstrap && f.deps.RecordExit != nil {
		f.deps.RecordExit(req.Argv, CommandTrace{
			Dir:              req.VirtualWD,
			Position:         req.Position,
			ResolvedName:     resolved.Name,
			ResolvedPath:     resolved.Path,
			ResolutionSource: resolved.Source,
			ExitCode:         exitCode,
			Duration:         time.Since(start),
		})
	}
	return finalEnv, err
}
