package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

type Pwd struct{}

type pwdOptions struct {
	logical     bool
	physical    bool
	showHelp    bool
	showVersion bool
}

func NewPwd() *Pwd {
	return &Pwd{}
}

func (c *Pwd) Name() string {
	return "pwd"
}

func (c *Pwd) Run(ctx context.Context, inv *Invocation) error {
	opts, err := parsePwdArgs(inv)
	if err != nil {
		return err
	}
	if opts.showHelp {
		_, _ = io.WriteString(inv.Stdout, pwdHelpText)
		return nil
	}
	if opts.showVersion {
		_, _ = io.WriteString(inv.Stdout, pwdVersionText)
		return nil
	}

	cwd, err := resolvePwdOutput(ctx, inv, opts)
	if err != nil {
		return exitf(inv, 1, "pwd: failed to get current directory: %s", pwdErrorDetail(err))
	}
	if _, err := fmt.Fprintln(inv.Stdout, cwd); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

func parsePwdArgs(inv *Invocation) (pwdOptions, error) {
	opts := pwdOptions{}
	args := append([]string(nil), inv.Args...)

	for len(args) > 0 {
		arg := args[0]
		switch {
		case arg == "--":
			args = args[1:]
			goto done
		case arg == "--help":
			opts.showHelp = true
			return opts, nil
		case arg == "--version":
			opts.showVersion = true
			return opts, nil
		case strings.HasPrefix(arg, "--"):
			match, err := matchPwdLongOption(inv, strings.TrimPrefix(arg, "--"))
			if err != nil {
				return pwdOptions{}, err
			}
			switch match {
			case "logical":
				opts.logical = true
			case "physical":
				opts.physical = true
			case "help":
				opts.showHelp = true
				return opts, nil
			case "version":
				opts.showVersion = true
				return opts, nil
			}
			args = args[1:]
		case arg == "-" || !strings.HasPrefix(arg, "-"):
			goto done
		default:
			args = args[1:]
			for _, ch := range arg[1:] {
				switch ch {
				case 'L':
					opts.logical = true
				case 'P':
					opts.physical = true
				default:
					return pwdOptions{}, pwdUsageError(inv, fmt.Sprintf("pwd: invalid option -- '%c'", ch))
				}
			}
		}
	}

done:
	if len(args) != 0 {
		return pwdOptions{}, exitf(inv, 1, "pwd: unexpected arguments")
	}
	return opts, nil
}

func matchPwdLongOption(inv *Invocation, name string) (string, error) {
	candidates := []string{"help", "logical", "physical", "version"}
	for _, candidate := range candidates {
		if candidate == name {
			return candidate, nil
		}
	}

	var matches []string
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, name) {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return "", pwdUsageError(inv, fmt.Sprintf("pwd: unrecognized option '--%s'", name))
	case 1:
		return matches[0], nil
	default:
		return "", pwdUsageError(inv, fmt.Sprintf("pwd: option '--%s' is ambiguous", name))
	}
}

func pwdUsageError(inv *Invocation, message string) error {
	return exitf(inv, 1, "%s\nTry 'pwd --help' for more information.", message)
}

func resolvePwdOutput(ctx context.Context, inv *Invocation, opts pwdOptions) (string, error) {
	useLogical := opts.logical || (!opts.physical && inv != nil && inv.Env["POSIXLY_CORRECT"] != "")
	if opts.physical || !useLogical {
		return pwdPhysicalPath(ctx, inv)
	}
	return pwdLogicalPath(ctx, inv)
}

func pwdPhysicalPath(ctx context.Context, inv *Invocation) (string, error) {
	if inv == nil || inv.FS == nil {
		return "/", nil
	}
	return inv.FS.Realpath(ctx, ".")
}

func pwdLogicalPath(ctx context.Context, inv *Invocation) (string, error) {
	if inv != nil {
		if candidate, ok := inv.Env["PWD"]; ok && pwdLooksReasonable(ctx, inv, candidate) {
			return candidate, nil
		}
	}
	return pwdPhysicalPath(ctx, inv)
}

func pwdLooksReasonable(ctx context.Context, inv *Invocation, candidate string) bool {
	if !path.IsAbs(candidate) {
		return false
	}
	for piece := range strings.SplitSeq(candidate, "/") {
		if piece == "." || piece == ".." {
			return false
		}
	}
	return pwdMatchesCurrentDir(ctx, inv, candidate)
}

func pwdMatchesCurrentDir(ctx context.Context, inv *Invocation, candidate string) bool {
	if inv == nil || inv.FS == nil {
		return false
	}

	candidateInfo, candidateErr := inv.FS.Stat(ctx, candidate)
	currentInfo, currentErr := inv.FS.Stat(ctx, ".")
	if candidateErr == nil && currentErr == nil && os.SameFile(candidateInfo, currentInfo) {
		return true
	}

	candidateReal, candidateRealErr := inv.FS.Realpath(ctx, candidate)
	currentReal, currentRealErr := inv.FS.Realpath(ctx, ".")
	if candidateRealErr != nil || currentRealErr != nil {
		return false
	}
	return candidateReal == currentReal
}

func pwdErrorDetail(err error) string {
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Err != nil {
		err = exitErr.Err
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr.Err != nil {
		return pathErr.Err.Error()
	}
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

const pwdHelpText = `Usage: pwd [OPTION]...
Print the full filename of the current working directory.

  -L, --logical   use PWD from environment, even if it contains symlinks
  -P, --physical  avoid all symlinks
      --help      display this help and exit
      --version   output version information and exit
`

const pwdVersionText = `pwd (gbash)
`

var _ Command = (*Pwd)(nil)
