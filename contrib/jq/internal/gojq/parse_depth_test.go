package gojq_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/contrib/jq/internal/gojq"
)

func TestParseRejectsDeepNesting(t *testing.T) {
	t.Parallel()

	src := strings.Repeat("(", 1_000_000) + "1" + strings.Repeat(")", 1_000_000)
	_, err := gojq.Parse(src)
	var depthErr *gojq.DepthLimitError
	if !errors.As(err, &depthErr) {
		t.Fatalf("Parse() error = %v, want DepthLimitError", err)
	}
	if _, err := gojq.Parse(strings.Repeat("[", 100) + "1" + strings.Repeat("]", 100)); err != nil {
		t.Fatalf("Parse(shallow) error = %v", err)
	}
}

func TestCompileRejectsDeepQueries(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		strings.Repeat("1+", 1_000_000) + "1",
		strings.Repeat(".|", 1_000_000) + ".",
		strings.Repeat("-", 1_000_000) + "1",
		strings.Repeat("try ", 1_000_000) + "1",
		"." + strings.Repeat(".a", 1_000_000),
		"." + strings.Repeat(".a", 1_000_000) + " = 1",
		strings.Repeat("1 as $x | ", 1_000_000) + "1",
		strings.Repeat("def f: ", 1_000_000) + "1" + strings.Repeat(";", 1_000_000) + " 1",
		strings.Repeat("reduce . as $x (0; ", 100_000) + "1" + strings.Repeat(")", 100_000),
		strings.Repeat("{a:", 1_000_000) + "1" + strings.Repeat("}", 1_000_000),
	} {
		q, err := gojq.Parse(src)
		if err != nil {
			continue
		}
		if _, err := gojq.Compile(q); err == nil {
			t.Errorf("Compile(%.20q...) succeeded, want depth error", src)
		}
	}
}
