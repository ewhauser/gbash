package builtins

import (
	"bufio"
	"context"
	"fmt"
	"io"
	stdfs "io/fs"
	"reflect"
	"time"
)

func baseEncodingCommandSpec(name, about string) CommandSpec {
	return CommandSpec{
		Name:  name,
		About: about,
		Usage: name + " [OPTION]... [FILE]",
		Options: []OptionSpec{
			{Name: "decode", Short: 'd', ShortAliases: []rune{'D'}, Long: "decode", Help: "decode data"},
			{Name: "ignore-garbage", Short: 'i', Long: "ignore-garbage", Help: "when decoding, ignore non-alphabetic characters"},
			{Name: "wrap", Short: 'w', Long: "wrap", ValueName: "COLS", Arity: OptionRequiredValue, Help: "wrap encoded lines after COLS character (default 76, 0 to disable wrapping)"},
		},
		Args: []ArgSpec{
			{Name: "file", ValueName: "FILE"},
		},
		Parse: ParseConfig{
			InferLongOptions:         true,
			GroupShortOptions:        true,
			ShortOptionValueAttached: true,
			LongOptionValueEquals:    true,
			AutoHelp:                 true,
			AutoVersion:              true,
		},
		HelpRenderer: func(w io.Writer, spec CommandSpec) error {
			_, err := io.WriteString(w, spec.About+"\n\nUsage: "+spec.Usage+"\n\nOptions:\n  -d, --decode           decode data\n  -i, --ignore-garbage   when decoding, ignore non-alphabetic characters\n  -w, --wrap=COLS        wrap encoded lines after COLS character (default 76, 0 to disable wrapping)\n  -h, --help             display this help and exit\n      --version          output version information and exit\n")
			return err
		},
	}
}

func promptOverwrite(inv *Invocation, commandName, destDisplay string, reader *bufio.Reader) (bool, error) {
	if inv != nil && inv.Stderr != nil {
		if _, err := fmt.Fprintf(inv.Stderr, "%s: overwrite %s? ", commandName, quoteGNUOperand(destDisplay)); err != nil {
			return false, &ExitError{Code: 1, Err: err}
		}
		if flusher, ok := inv.Stderr.(interface{ Flush() error }); ok {
			if err := flusher.Flush(); err != nil {
				return false, &ExitError{Code: 1, Err: err}
			}
		}
	}

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, exitf(inv, 1, "%s: Failed to read from standard input", commandName)
	}
	if line == "" {
		return false, nil
	}
	return line[0] == 'y' || line[0] == 'Y', nil
}

func runByteTransformCommand[T any](
	ctx context.Context,
	inv *Invocation,
	matches *ParsedCommand,
	spec *CommandSpec,
	commandName string,
	parse func(*Invocation, *ParsedCommand) (T, error),
	files func(T) []string,
	transform func([]byte, *T) []byte,
) error {
	if matches.Has("help") {
		return RenderCommandHelp(inv.Stdout, spec)
	}
	if matches.Has("version") {
		return RenderCommandVersion(inv.Stdout, spec)
	}

	opts, err := parse(inv, matches)
	if err != nil {
		return err
	}

	inputs := files(opts)
	if len(inputs) == 0 {
		inputs = []string{"-"}
	}

	var hadErrors bool
	for _, name := range inputs {
		data, err := readTransformInput(ctx, inv, name)
		if err != nil {
			hadErrors = true
			if _, writeErr := fmt.Fprintf(inv.Stderr, "%s: %s: %s\n", commandName, name, readAllErrorText(err)); writeErr != nil {
				return &ExitError{Code: 1, Err: writeErr}
			}
			continue
		}
		if _, err := inv.Stdout.Write(transform(data, &opts)); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
	}

	if hadErrors {
		return &ExitError{Code: 1}
	}
	return nil
}

func readTransformInput(ctx context.Context, inv *Invocation, name string) ([]byte, error) {
	if name == "-" {
		return readAllStdin(ctx, inv)
	}
	read, _, err := readAllFile(ctx, inv, name)
	if err != nil {
		return nil, err
	}
	return read, nil
}

func fileInfoAccessTime(
	info stdfs.FileInfo,
	timespec func(reflect.Value) (time.Time, bool),
	uintField func(reflect.Value) uint64,
) (time.Time, bool) {
	if info == nil {
		return time.Time{}, false
	}
	sys := reflect.ValueOf(info.Sys())
	if !sys.IsValid() {
		return time.Time{}, false
	}
	if sys.Kind() == reflect.Pointer {
		if sys.IsNil() {
			return time.Time{}, false
		}
		sys = sys.Elem()
	}
	if sys.Kind() != reflect.Struct {
		return time.Time{}, false
	}
	if field := sys.FieldByName("Atim"); field.IsValid() {
		return timespec(field)
	}
	if field := sys.FieldByName("Atimespec"); field.IsValid() {
		return timespec(field)
	}
	if sec := sys.FieldByName("Atime"); sec.IsValid() {
		nsec := sys.FieldByName("AtimeNsec")
		return time.Unix(int64(uintField(sec)), int64(uintField(nsec))), true
	}
	return time.Time{}, false
}
