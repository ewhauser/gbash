package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	ctx := context.Background()
	opts, err := parseOptions()
	if err != nil {
		fatalf("parse options: %v", err)
	}
	manifest, err := loadManifest()
	if err != nil {
		fatalf("load manifest: %v", err)
	}
	if err := run(ctx, manifest, &opts); err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "gbash-gnu: "+format+"\n", args...)
	os.Exit(1)
}
