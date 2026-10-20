package syntax

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

// Each level of "$( ((" or "$((" used to make every probe re-parse all the
// levels below it: depth 1600 took 30 s, and the time grew quadratically.
// With the probe memo, depth 3200 parses in about 0.1 s. What still grows
// faster than linearly is copying the source text that each level's node
// keeps, which is cheap.
func TestParseNestedParenAmbiguityIsFast(t *testing.T) {
	t.Parallel()
	const n = 3200
	inputs := map[string]string{
		"unclosed": strings.Repeat("$( (( ", n) + "1" + strings.Repeat(" ))", n),
		"closed":   strings.Repeat("$( (( ", n) + "1" + strings.Repeat(" )) )", n),
		"arith":    strings.Repeat("$(( ", n) + "1" + strings.Repeat(" ))", n),
	}
	for name, src := range inputs {
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = NewParser().Parse(strings.NewReader(src), "")
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: parse at depth %d did not finish within 10s", name, n)
		}
	}
}

// The probe memo must not change what a parse returns.
func TestProbeMemoMatchesFreshParse(t *testing.T) {
	pieces := []string{
		"((", "$((", "$(", "(", ")", "))", " ", "1", "echo a", "x",
		";", "\n", "`", "'", "\"", "+", "<<E\n", "E\n", "{", "}", "$[", "]", "\\",
	}
	rng := rand.New(rand.NewSource(3))
	n := 3000
	if testing.Short() {
		n = 300
	}
	var srcs []string
	for range n {
		var b strings.Builder
		for range 2 + rng.Intn(16) {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		srcs = append(srcs, b.String())
	}
	langs := []LangVariant{LangBash, LangPOSIX}
	want := make(map[string]string)
	probeMemoOff = true
	for _, src := range srcs {
		for _, lang := range langs {
			want[lang.String()+src] = parseForDiff(lang, strings.NewReader(src))
		}
	}
	probeMemoOff = false
	for _, src := range srcs {
		for _, lang := range langs {
			if got := parseForDiff(lang, strings.NewReader(src)); got != want[lang.String()+src] {
				t.Fatalf("%v: parse of %q differs with the probe memo:\nwithout:\n%s\nwith:\n%s", lang, src, want[lang.String()+src], got)
			}
		}
	}
}
