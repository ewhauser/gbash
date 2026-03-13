package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

func main() {
	if err := run(context.Background(), os.Stdin, os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args []string) error {
	opts, err := parseCLI(args)
	if err != nil {
		return err
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}

	interactivePause := isTerminalReader(stdin) && isTerminalWriter(stdout) && !opts.quiet
	colorEnabled := isTerminalWriter(stdout) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	if opts.pause {
		interactivePause = true
	}
	if opts.noPause {
		interactivePause = false
	}
	if opts.color {
		colorEnabled = true
	}
	if opts.noColor {
		colorEnabled = false
	}

	return runDemo(ctx, stdin, stdout, stderr, demoOptions{
		quiet: opts.quiet,
		pause: interactivePause,
		color: colorEnabled,
	})
}

type cliOptions struct {
	quiet   bool
	pause   bool
	noPause bool
	color   bool
	noColor bool
}

type demoOptions struct {
	quiet bool
	pause bool
	color bool
}

func parseCLI(args []string) (cliOptions, error) {
	var opts cliOptions

	fs := flag.NewFlagSet("transactional-workspaces", flag.ContinueOnError)
	fs.BoolVar(&opts.quiet, "quiet", false, "print a compact version of the demo")
	fs.BoolVar(&opts.quiet, "q", false, "print a compact version of the demo")
	fs.BoolVar(&opts.pause, "pause", false, "pause between major demo beats")
	fs.BoolVar(&opts.noPause, "no-pause", false, "disable interactive pauses")
	fs.BoolVar(&opts.color, "color", false, "force ANSI color output")
	fs.BoolVar(&opts.noColor, "no-color", false, "disable ANSI color output")

	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}
	if opts.pause && opts.noPause {
		return cliOptions{}, fmt.Errorf("--pause and --no-pause cannot be used together")
	}
	if opts.color && opts.noColor {
		return cliOptions{}, fmt.Errorf("--color and --no-color cannot be used together")
	}
	if fs.NArg() != 0 {
		return cliOptions{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return opts, nil
}

func isTerminalReader(reader io.Reader) bool {
	stream, ok := reader.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(int(stream.Fd()))
}

func isTerminalWriter(writer io.Writer) bool {
	stream, ok := writer.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(int(stream.Fd()))
}
