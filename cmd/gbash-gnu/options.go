package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func parseOptions() (options, error) {
	var opts options
	fs := flag.NewFlagSet("gbash-gnu", flag.ContinueOnError)
	fs.StringVar(&opts.cacheDir, "cache-dir", ".cache/gnu", "cache directory for GNU sources and results")
	fs.StringVar(&opts.gbashBin, "gbash-bin", "", "path to the gbash binary under test")
	fs.StringVar(&opts.utils, "utils", strings.TrimSpace(os.Getenv("GNU_UTILS")), "comma or space separated utility list")
	fs.StringVar(&opts.tests, "tests", strings.TrimSpace(os.Getenv("GNU_TESTS")), "comma or newline separated explicit GNU test files")
	fs.StringVar(&opts.resultsDir, "results-dir", strings.TrimSpace(os.Getenv("GNU_RESULTS_DIR")), "directory to write summary.json, logs, and published report assets")
	fs.StringVar(&opts.preparedBuildArchive, "prepared-build-archive", strings.TrimSpace(os.Getenv("GNU_PREPARED_BUILD_ARCHIVE")), "path to a prepared GNU build archive to restore before running tests")
	fs.StringVar(&opts.writePreparedBuildArchive, "write-prepared-build-archive", strings.TrimSpace(os.Getenv("GNU_WRITE_PREPARED_BUILD_ARCHIVE")), "write a prepared GNU build archive to this path, then exit")
	fs.BoolVar(&opts.setupOnly, "setup", false, "download and extract the pinned GNU source tree, then exit")
	fs.BoolVar(&opts.keepWorkdir, "keep-workdir", os.Getenv("GNU_KEEP_WORKDIR") == "1", "preserve the per-run workdir")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if opts.setupOnly && strings.TrimSpace(opts.writePreparedBuildArchive) != "" {
		return options{}, fmt.Errorf("--setup and --write-prepared-build-archive cannot be combined")
	}
	if strings.TrimSpace(opts.preparedBuildArchive) != "" && strings.TrimSpace(opts.writePreparedBuildArchive) != "" {
		return options{}, fmt.Errorf("--prepared-build-archive and --write-prepared-build-archive cannot be combined")
	}
	if !opts.setupOnly && strings.TrimSpace(opts.writePreparedBuildArchive) == "" && strings.TrimSpace(opts.gbashBin) == "" {
		return options{}, fmt.Errorf("--gbash-bin is required unless --setup or --write-prepared-build-archive is used")
	}
	return opts, nil
}

func parseList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}
