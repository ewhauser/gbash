package python

import (
	"slices"
	"strings"
	"testing"

	gbruntime "github.com/ewhauser/gbash"
)

func TestRegisterAddsPythonAndPython3Commands(t *testing.T) {
	t.Parallel()

	registry := newPythonRegistry(t)
	for _, name := range []string{"python", "python3"} {
		if !slices.Contains(registry.Names(), name) {
			t.Fatalf("Names() missing %q: %v", name, registry.Names())
		}
	}
	for _, name := range []string{"python", "python3"} {
		if slices.Contains(gbruntime.DefaultRegistry().Names(), name) {
			t.Fatalf("DefaultRegistry() unexpectedly contains %q", name)
		}
	}
}

func TestRewritePrintCallsRewritesBarePrintOnly(t *testing.T) {
	t.Parallel()

	source := "" +
		"print('top')\n" +
		"message = \"print('inside string')\"\n" +
		"obj.print('method')\n" +
		"def print(value):\n" +
		"    return value\n"

	rewritten := rewritePrintCalls(source)
	if !strings.Contains(rewritten, "__gbash_print('top')") {
		t.Fatalf("rewritePrintCalls() = %q, want bare print rewritten", rewritten)
	}
	if strings.Contains(rewritten, "obj.__gbash_print") {
		t.Fatalf("rewritePrintCalls() = %q, want method access preserved", rewritten)
	}
	if strings.Contains(rewritten, "def __gbash_print") {
		t.Fatalf("rewritePrintCalls() = %q, want function definition preserved", rewritten)
	}
	if !strings.Contains(rewritten, "\"print('inside string')\"") {
		t.Fatalf("rewritePrintCalls() = %q, want string literal preserved", rewritten)
	}
}
