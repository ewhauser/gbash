// Copyright (c) 2016, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package syntax

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkParse(b *testing.B) {
	b.ReportAllocs()
	src := "" +
		strings.Repeat("\n\n\t\t        \n", 10) +
		"# " + strings.Repeat("foo bar ", 10) + "\n" +
		strings.Repeat("longlit_", 10) + "\n" +
		"'" + strings.Repeat("foo bar ", 10) + "'\n" +
		`"` + strings.Repeat("foo bar ", 10) + `"` + "\n" +
		strings.Repeat("aa bb cc dd; ", 6) +
		"a() { (b); { c; }; }; $(d; `e`)\n" +
		"foo=bar; a=b; c=d$foo${bar}e $simple ${complex:-default}\n" +
		"if a; then while b; do for c in d e; do f; done; done; fi\n" +
		"a | b && c || d | e && g || f\n" +
		"foo >a <b <<<c 2>&1 <<EOF\n" +
		strings.Repeat("somewhat long heredoc line\n", 10) +
		"EOF" +
		""
	p := NewParser(KeepComments(true))
	in := strings.NewReader(src)
	for b.Loop() {
		if _, err := p.Parse(in, ""); err != nil {
			b.Fatal(err)
		}
		in.Reset(src)
	}
}

func BenchmarkParseBenchmarkFiles(b *testing.B) {
	cases := []struct {
		name string
		path string
	}{
		{
			name: "nvm",
			path: filepath.Join("testdata", "benchmarks", "files", "nvm.sh"),
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			src, err := os.ReadFile(tc.path)
			if err != nil {
				b.Fatalf("ReadFile(%q) error = %v", tc.path, err)
			}

			parser := NewParser(KeepComments(true))
			reader := bytes.NewReader(src)

			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				reader.Reset(src)
				if _, err := parser.Parse(reader, filepath.Base(tc.path)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPrint(b *testing.B) {
	b.ReportAllocs()
	prog := parsePath(b, canonicalPath)
	printer := NewPrinter()
	for b.Loop() {
		if err := printer.Print(io.Discard, prog); err != nil {
			b.Fatal(err)
		}
	}
}
