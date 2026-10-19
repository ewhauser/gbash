package gojq_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/contrib/jq/internal/gojq"
)

func deepArray(depth int) any {
	var v any = 0
	for range depth {
		v = []any{v}
	}
	return v
}

func TestMarshalReportsDepthLimit(t *testing.T) {
	if _, err := gojq.Marshal(deepArray(gojq.MaxDepth)); err != nil {
		t.Fatalf("Marshal(depth %d) error = %v, want nil", gojq.MaxDepth, err)
	}
	_, err := gojq.Marshal(deepArray(gojq.MaxDepth + 1))
	var depthErr *gojq.DepthLimitError
	if !errors.As(err, &depthErr) {
		t.Fatalf("Marshal(depth %d) error = %v, want DepthLimitError", gojq.MaxDepth+1, err)
	}
}

func TestRunStopsAtDepthLimit(t *testing.T) {
	for _, src := range []string{
		". == .",
		"tojson",
		"contains(.)",
		"flatten",
		"try (. < .) catch \"caught\"",
		"(. == .), 1",
	} {
		t.Run(src, func(t *testing.T) {
			query, err := gojq.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			iter := query.Run(deepArray(3 * gojq.MaxDepth))
			v, ok := iter.Next()
			var depthErr *gojq.DepthLimitError
			if !ok || !errors.As(asError(v), &depthErr) {
				t.Fatalf("first value = %v, want DepthLimitError", v)
			}
			if v, ok := iter.Next(); ok {
				t.Fatalf("iterator continued after depth error with %v", v)
			}
		})
	}
}

func TestSetpathRejectsLongPaths(t *testing.T) {
	path := make([]any, gojq.MaxDepth+1)
	for i := range path {
		path[i] = 0
	}
	query, err := gojq.Parse("setpath($p; 1)")
	if err != nil {
		t.Fatal(err)
	}
	code, err := gojq.Compile(query, gojq.WithVariables([]string{"$p"}))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := code.Run(nil, path).Next()
	if err := asError(v); err == nil || !strings.Contains(err.Error(), "exceeds depth limit") {
		t.Fatalf("setpath value = %v, want depth limit error", v)
	}
}

func asError(v any) error {
	err, _ := v.(error)
	return err
}
