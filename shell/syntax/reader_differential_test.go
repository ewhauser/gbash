package syntax

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"
	"testing/iotest"
)

// parseForDiff parses src from r and returns a comparable summary of the
// result: the printed AST and the error text.
func parseForDiff(lang LangVariant, r io.Reader) string {
	f, err := NewParser(Variant(lang)).Parse(r, "")
	var out bytes.Buffer
	if f != nil {
		if perr := NewPrinter().Print(&out, f); perr != nil {
			fmt.Fprintf(&out, "<print error: %v>", perr)
		}
	}
	return fmt.Sprintf("err=%v\n%s", err, out.String())
}

func checkReaderDifferential(t *testing.T, lang LangVariant, src string) {
	t.Helper()
	whole := parseForDiff(lang, strings.NewReader(src))
	chunked := parseForDiff(lang, iotest.OneByteReader(strings.NewReader(src)))
	if whole != chunked {
		t.Errorf("%v: parse of %q differs by reader:\nstring reader:\n%s\none-byte reader:\n%s", lang, src, whole, chunked)
	}
}

// Nested paren ambiguity probes used to drop the enclosing source view when
// an inner probe finished, so a chunked reader parsed differently from a
// string reader.
func TestParseReaderDifferential(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"(( $((1)) ((echo a); echo b); ))\necho tail\n",
		"echo $(( $((1)) ((echo a); echo b); ))\necho tail\n",
		"(( ((1)) ((echo a); echo b); ))\necho tail\n",
		"$( (( $( (( 1 )) ) )) )\n",
	} {
		for _, lang := range []LangVariant{LangBash, LangPOSIX} {
			checkReaderDifferential(t, lang, src)
		}
	}
	// Backquote recovery inside a Bash probe used to write through a view
	// of the shared replay buffer and corrupt the later probes.
	for _, src := range []string{
		"(( `\"` ))",
		"$((`\"\n`))",
		"$((+`\"`x<<E\n+1``\";",
	} {
		checkReaderDifferential(t, LangBash, src)
	}
}

// The alphabet leaves out backquotes: recovering a malformed backquote outside
// a probe still sees less source with a chunked reader (predates the probe
// replay buffer).
func TestParseReaderDifferentialGenerated(t *testing.T) {
	t.Parallel()
	pieces := []string{
		"((", "$((", "$(", "(", ")", "))", " ", "1", "echo a", "echo b",
		";", "\n", "'", "\"", "+", "x", "<<E\n", "E\n", "{", "}", "#",
	}
	rng := rand.New(rand.NewSource(1))
	n := 4000
	if testing.Short() {
		n = 500
	}
	for range n {
		var b strings.Builder
		for range 2 + rng.Intn(14) {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		src := b.String()
		for _, lang := range []LangVariant{LangBash, LangPOSIX} {
			checkReaderDifferential(t, lang, src)
		}
		if t.Failed() {
			return
		}
	}
}
