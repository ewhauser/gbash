package nodejs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	gbruntime "github.com/ewhauser/gbash"
)

func runNodeScript(t *testing.T, js string) *gbruntime.ExecutionResult {
	t.Helper()

	session := newNodeSession(t)
	writeSessionFile(t, session, "/home/agent/main.js", []byte(js))
	result, err := session.Exec(context.Background(), &gbruntime.ExecutionRequest{
		Script:  "nodejs /home/agent/main.js\necho after=$?\n",
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	return result
}

func TestNodeJSRejectsOversizedAllocations(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		js   string
	}{
		{name: "repeat", js: `'x'.repeat(100 * 1024 * 1024)`},
		{name: "repeat object receiver", js: `String.prototype.repeat.call({ toString() { return 'ab'; } }, 1e9)`},
		{name: "padStart", js: `'x'.padStart(1e9, 'y')`},
		{name: "padEnd", js: `'x'.padEnd(1e9)`},
		{name: "ArrayBuffer", js: `new ArrayBuffer(1e10)`},
		{name: "typed array length", js: `new Float64Array(2 ** 28)`},
		{name: "typed array from array-like", js: `new Uint8Array({ length: 1e10 })`},
		{name: "typed array static from", js: `Uint8Array.from({ length: 1e10 })`},
		{name: "subclassed typed array", js: `class Big extends Uint8Array {}; new Big(1e10)`},
		{name: "Buffer.alloc", js: `Buffer.alloc(1e9)`},
		{name: "Buffer.from array-like", js: `Buffer.from({ length: 1e10 })`},
		{name: "Buffer.concat length", js: `Buffer.concat([], 1e10)`},
		{name: "Array length", js: `new Array(1e9)`},
		{name: "Array call", js: `Array(2 ** 32 - 1)`},
		{name: "sparse array fill", js: `const a = []; a.length = 1e9; a.fill(0)`},
		{name: "sparse array join", js: `const a = []; a.length = 2 ** 32 - 1; a.join()`},
		{name: "array-like method", js: `Array.prototype.indexOf.call({ length: 1e12 }, 1)`},
		{name: "Array.from array-like", js: `Array.from({ length: 1e9 })`},
		{name: "split", js: `'x'.repeat(32 * 1024 * 1024).split('')`},
		{name: "join cumulative", js: `Array.from({ length: 100 }, () => 'x'.repeat(1 << 20)).join('')`},
		{name: "fill then join", js: `new Array(1 << 24).fill('xxxxxxxx').join('')`},
		{name: "stringify strings", js: `JSON.stringify(Array.from({ length: 100 }, () => 'x'.repeat(1 << 20)))`},
		{name: "stringify with array replacer", js: `JSON.stringify({ a: Array.from({ length: 100 }, () => 'x'.repeat(1 << 20)) }, ['a'])`},
		{name: "JSON.parse deep", js: `JSON.parse('['.repeat(4000000))`},
		{name: "eval deep", js: `eval('('.repeat(4000000))`},
		{name: "Function deep", js: `new Function('return ' + '['.repeat(4000000))`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runNodeScript(t, "try { "+tc.js+"; console.log('allocated') } catch (e) { console.log(e instanceof RangeError, e.message) }\n")
			if !strings.HasPrefix(result.Stdout, "true ") || !strings.Contains(result.Stdout, "exceeds the sandbox limit") {
				t.Fatalf("Stdout = %q, want catchable RangeError; stderr=%q", result.Stdout, result.Stderr)
			}
		})
	}
}

func TestNodeJSAllocationWrappersPreserveBehavior(t *testing.T) {
	t.Parallel()

	result := runNodeScript(t, strings.Join([]string{
		`console.log('ab'.repeat(3), 'x'.padStart(3, '-'), 'x'.padEnd(3, '+'))`,
		`console.log(new Uint8Array(4) instanceof Uint8Array, new Uint8Array([1, 2]).length, Uint8Array.from([1, 2, 3]).length)`,
		`console.log(new Uint8Array(1).constructor === Uint8Array, Buffer.from('a') instanceof Uint8Array, Uint8Array.BYTES_PER_ELEMENT, Uint8Array.name)`,
		`class Sub extends Uint8Array {}; console.log(new Sub(2) instanceof Sub, new ArrayBuffer(8).byteLength)`,
		`console.log(Buffer.alloc(3, 'a').toString(), Buffer.from('hi').toString(), Buffer.concat([Buffer.from('a'), Buffer.from('b')]).toString())`,
		`try { String.prototype.repeat.call(null, 2) } catch (e) { console.log(e instanceof TypeError) }`,
		`console.log([] instanceof Array, Array.isArray(new Array(3)), new Array(3).length, Array(1, 2).length, Array.from('ab').join(), Array.of(7)[0], [3, 1, 2].sort().join(''), 'a,b'.split(','), 'abc'.split('', 2))`,
		`class List extends Array {}; const l = new List(); l.push(1); console.log(l instanceof List, l.map((x) => x + 1) instanceof List, [1, 2].map((x) => x * 2).constructor === Array)`,
		`try { 'x'.repeat(-1) } catch (e) { console.log(e instanceof RangeError) }`,
	}, "\n")+"\n")
	want := "ababab --x x++\ntrue 2 3\ntrue true 1 Uint8Array\ntrue 8\naaa hi ab\ntrue\ntrue true 3 2 a,b 7 123 a,b a,b\ntrue true true\ntrue\nafter=0\n"
	if result.Stdout != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", result.Stdout, want, result.Stderr)
	}
}

// TestNodeJSStopsWhenHeapGrowthExceedsBudget is not parallel: the heap
// budget is measured process-wide, so concurrent tests would skew it.
//
//nolint:paralleltest // see above
func TestNodeJSStopsWhenHeapGrowthExceedsBudget(t *testing.T) {
	for _, tc := range []struct {
		name string
		js   string
	}{
		{name: "retained strings", js: `const a = []; for (let i = 0; i < 48; i++) a.push('x'.repeat(60 * 1024 * 1024) + i)`},
		{name: "pushed numbers", js: `const a = []; for (;;) a.push({ n: a.length })`},
	} {
		t.Run(tc.name, func(t *testing.T) { //nolint:paralleltest // see above
			result := runNodeScript(t, tc.js+"\n")
			if got, want := result.Stdout, "after=1\n"; got != want {
				t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "memory limit") {
				t.Fatalf("Stderr = %q, want memory limit error", result.Stderr)
			}
		})
	}
}

func TestNodeJSWatchdogStopsInfiniteLoopWithoutDeadline(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	stop := watchNodeExecution(context.Background(), vm, 200*time.Millisecond, nodeMaxHeapGrowthBytes)
	defer stop()

	start := time.Now()
	_, err := vm.RunString(`for (;;) {}`)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("loop ran for %s past a 200ms limit", elapsed)
	}
	var interrupted *goja.InterruptedError
	if !errors.As(err, &interrupted) {
		t.Fatalf("RunString() error = %v, want interrupt", err)
	}
	signal, ok := interrupted.Value().(interruptSignal)
	if !ok || !signal.limit || !errors.Is(signal.err, errNodeTimeLimit) {
		t.Fatalf("interrupt value = %#v, want time limit", interrupted.Value())
	}
}

func TestNodeJSWrappedBuiltinsKeepSemantics(t *testing.T) {
	t.Parallel()

	result := runNodeScript(t, `
console.log([1, null, undefined, 'a', [2, 3]].join('-'));
console.log([1, 2].join());
console.log(JSON.stringify({ b: 1, a: [1, 'x'], c: { a: 2, z: 3 } }, ['a', 'b']));
console.log(JSON.stringify({ a: 1, b: 2 }, (k, v) => (k === 'b' ? undefined : v), 1));
console.log(JSON.parse('{"x": "[[[[", "y": [1, {"z": 2}]}').y[1].z);
console.log(eval('1 + 2'), new Function('a', 'return a * 2')(21), (function () {}).constructor === Function);
`)
	want := "1---a-2,3\n1,2\n{\"a\":[1,\"x\"],\"b\":1}\n{\n \"a\": 1\n}\n2\n3 42 true\nafter=0\n"
	if result.Stdout != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", result.Stdout, want, result.Stderr)
	}
}

func TestNodeJSRejectsDeeplyNestedModuleSource(t *testing.T) {
	t.Parallel()

	result := runNodeScript(t, "const x = "+strings.Repeat("[", 4000000)+";\n")
	if !strings.Contains(result.Stderr, "nesting exceeds") || strings.Contains(result.Stdout, "after=0") {
		t.Fatalf("stdout=%q stderr=%.300q, want nesting error", result.Stdout, result.Stderr)
	}
}
