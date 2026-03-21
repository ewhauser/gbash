package builtins

import (
	"context"
	"errors"
	"fmt"
	stdfs "io/fs"
	"path"
	"strconv"
	"strings"
)

type Mkfifo struct{}

type mkfifoOptions struct {
	mode    stdfs.FileMode
	modeSet bool
}

var errMkfifoPermissionBits = errors.New("mkfifo mode must specify only permission bits")

func NewMkfifo() *Mkfifo {
	return &Mkfifo{}
}

func (c *Mkfifo) Name() string {
	return "mkfifo"
}

func (c *Mkfifo) Run(ctx context.Context, inv *Invocation) error {
	return RunCommand(ctx, c, inv)
}

func (c *Mkfifo) Spec() CommandSpec {
	return CommandSpec{
		Name:      "mkfifo",
		About:     "Create named pipes (FIFOs) with the given names",
		Usage:     "mkfifo [OPTION]... NAME...",
		AfterHelp: "Each MODE is of the form [ugoa]*([-+=]([rwxXst]*|[ugo]))+|[-+=]?[0-7]+.",
		Options: []OptionSpec{
			{Name: "mode", Short: 'm', Long: "mode", Arity: OptionRequiredValue, ValueName: "MODE", Help: "set file permission bits"},
			{Name: "selinux", Short: 'Z', Help: "set the SELinux security context to the default type"},
			{Name: "context", Long: "context", Arity: OptionOptionalValue, ValueName: "CTX", Help: "like -Z, or set the SELinux or SMACK security context to CTX"},
		},
		Args: []ArgSpec{
			{Name: "name", ValueName: "NAME", Repeatable: true, Required: true},
		},
		Parse: ParseConfig{
			InferLongOptions:         true,
			GroupShortOptions:        true,
			ShortOptionValueAttached: true,
			LongOptionValueEquals:    true,
			AutoHelp:                 true,
			AutoVersion:              true,
		},
	}
}

func (c *Mkfifo) RunParsed(ctx context.Context, inv *Invocation, matches *ParsedCommand) error {
	opts, args, err := parseMkfifoMatches(inv, matches)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return exitf(inv, 1, "mkfifo: missing operand")
	}

	createMode := stdfs.FileMode(0o666) &^ chmodCurrentUmask(inv)
	if opts.modeSet {
		createMode = opts.mode
	}

	for _, name := range args {
		abs := allowPath(inv, name)
		if err := mkfifoPath(ctx, inv, name, abs, createMode); err != nil {
			return err
		}
	}
	return nil
}

func parseMkfifoMatches(inv *Invocation, matches *ParsedCommand) (mkfifoOptions, []string, error) {
	opts := mkfifoOptions{}
	if matches.Has("mode") {
		mode, err := parseMkfifoMode(matches.Value("mode"))
		if err != nil {
			if errors.Is(err, errMkfifoPermissionBits) {
				return mkfifoOptions{}, nil, exitf(inv, 1, "mkfifo: mode must specify only file permission bits")
			}
			return mkfifoOptions{}, nil, exitf(inv, 1, "mkfifo: invalid mode %q", matches.Value("mode"))
		}
		opts.mode = mode
		opts.modeSet = true
	}
	return opts, matches.Args("name"), nil
}

func parseMkfifoMode(spec string) (stdfs.FileMode, error) {
	spec = strings.TrimSpace(spec)
	if spec != "" && spec[0] >= '0' && spec[0] <= '7' {
		value, err := strconv.ParseUint(spec, 8, 32)
		if err == nil {
			if value > 0o777 {
				return 0, errMkfifoPermissionBits
			}
			return stdfs.FileMode(value), nil
		}
	}
	mode, err := computeChmodModeWithUmask(0o666, spec, 0, false)
	if err != nil {
		return 0, err
	}
	if mode&(stdfs.ModeSetuid|stdfs.ModeSetgid|stdfs.ModeSticky) != 0 {
		return 0, errMkfifoPermissionBits
	}
	return mode & 0o777, nil
}

func mkfifoPath(ctx context.Context, inv *Invocation, raw, abs string, perm stdfs.FileMode) error {
	if _, err := inv.FS.Lstat(ctx, abs); err == nil {
		return exitf(inv, 1, "mkfifo: cannot create fifo %s: File exists", quoteGNUOperand(raw))
	} else if !errors.Is(err, stdfs.ErrNotExist) {
		return &ExitError{Code: 1, Err: err}
	}

	parent := path.Dir(abs)
	info, err := inv.FS.Stat(ctx, parent)
	if err != nil {
		if errors.Is(err, stdfs.ErrNotExist) {
			return exitf(inv, 1, "mkfifo: cannot create fifo %s: No such file or directory", quoteGNUOperand(raw))
		}
		return &ExitError{Code: 1, Err: err}
	}
	if !info.IsDir() {
		return exitf(inv, 1, "mkfifo: cannot create fifo %s: Not a directory", quoteGNUOperand(raw))
	}

	if err := inv.FS.Mkfifo(ctx, abs, perm); err != nil {
		return exitf(inv, 1, "mkfifo: cannot create fifo %s: %s", quoteGNUOperand(raw), mkfifoErrorText(err))
	}
	return nil
}

func mkfifoErrorText(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, stdfs.ErrExist):
		return "File exists"
	case errors.Is(err, stdfs.ErrNotExist):
		return "No such file or directory"
	case errors.Is(err, stdfs.ErrPermission):
		return "Permission denied"
	case errors.Is(err, stdfs.ErrInvalid):
		return "Invalid argument"
	default:
		return fmt.Sprint(err)
	}
}

var _ Command = (*Mkfifo)(nil)
var _ SpecProvider = (*Mkfifo)(nil)
var _ ParsedRunner = (*Mkfifo)(nil)
