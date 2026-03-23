package main

import (
	"context"
	"fmt"
	"os"

	gbasheval "github.com/ewhauser/gbash/examples/gbash-eval/internal"
)

func main() {
	if err := gbasheval.RunCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
