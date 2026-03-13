package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(context.Background(), os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	opts, err := parseCLI(args)
	if err != nil {
		return err
	}
	return runDemo(ctx, stdout, stderr, opts)
}

type cliOptions struct {
	quiet bool
}

func parseCLI(args []string) (cliOptions, error) {
	var opts cliOptions

	fs := flag.NewFlagSet("transactional-workspaces", flag.ContinueOnError)
	fs.BoolVar(&opts.quiet, "quiet", false, "print a compact version of the demo")
	fs.BoolVar(&opts.quiet, "q", false, "print a compact version of the demo")

	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}
	if fs.NArg() != 0 {
		return cliOptions{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return opts, nil
}
