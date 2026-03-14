package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	opts, err := parseOptions()
	if err != nil {
		fatalf("parse options: %v", err)
	}
	manifest, err := loadManifest()
	if err != nil {
		fatalf("load manifest: %v", err)
	}
	if err := run(ctx, manifest, &opts); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			fatalf("compatibility run interrupted")
		}
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "gbash-gnu: "+format+"\n", args...)
	os.Exit(1)
}
