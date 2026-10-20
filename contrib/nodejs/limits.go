package nodejs

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"runtime/metrics"
	"time"

	"github.com/dop251/goja"
)

const (
	// nodeMaxAllocBytes caps any single string, buffer or typed array a
	// script can request in one call, so one expression cannot allocate
	// gigabytes before the heap watchdog gets a chance to sample.
	nodeMaxAllocBytes = 64 << 20

	// nodeMaxArrayLength caps the length of arrays that goja builtins walk
	// natively. goja only checks for interrupts between bytecode
	// instructions, so a native loop over a huge sparse array (for example
	// `new Array(1e9).fill(0)`) cannot be stopped once it starts.
	nodeMaxArrayLength = 1 << 24

	// nodeMaxHeapGrowthBytes bounds how far the process heap may grow while
	// a script runs. goja has no allocation accounting of its own, so this
	// is measured process-wide and confirmed after a forced GC.
	nodeMaxHeapGrowthBytes = 1 << 30

	// nodeDefaultMaxDuration bounds execution when the request has no
	// deadline, since goja never consults the shell's loop or command
	// budgets.
	nodeDefaultMaxDuration = 60 * time.Second

	// nodeMaxNesting caps bracket nesting in source text and JSON. goja's
	// parsers recurse once per level, and overflowing the Go stack is a
	// fatal error for the whole process.
	nodeMaxNesting = 10000

	nodeHeapSampleInterval = 10 * time.Millisecond
	nodeHeapMetric         = "/memory/classes/heap/objects:bytes"
)

var (
	errNodeTimeLimit   = fmt.Errorf("execution exceeded the %s time limit", nodeDefaultMaxDuration)
	errNodeMemoryLimit = fmt.Errorf("execution exceeded the %d MiB memory limit", nodeMaxHeapGrowthBytes>>20)
)

// nodeAllocLimitsScript wraps the builtins that allocate a caller-chosen
// size up front or loop natively over a caller-chosen length.
const nodeAllocLimitsScript = `(function (maxBytes, maxArrayLength, maxNesting) {
	"use strict";
	const tooLarge = (what) => new RangeError(what + " exceeds the sandbox limit of " + maxBytes + " bytes");
	const tooLong = (what) => new RangeError(what + " exceeds the sandbox limit of " + maxArrayLength + " elements");
	const define = (obj, name, fn) => Object.defineProperty(obj, name, { value: fn, writable: true, configurable: true, enumerable: false });

	const repeat = String.prototype.repeat;
	define(String.prototype, "repeat", {
		repeat(count) {
			if (this == null) return repeat.call(this, count);
			const s = String(this);
			const n = Number(count);
			if (s.length * n > maxBytes) throw tooLarge("String.prototype.repeat result");
			return repeat.call(s, n);
		},
	}.repeat);
	for (const name of ["padStart", "padEnd"]) {
		const pad = String.prototype[name];
		define(String.prototype, name, {
			[name](targetLength, padString) {
				if (this == null) return pad.call(this, targetLength, padString);
				const s = String(this);
				const n = Number(targetLength);
				if (n > maxBytes && n > s.length) throw tooLarge("String.prototype." + name + " result");
				return pad.call(s, n, padString);
			},
		}[name]);
	}

	const OriginalArrayBuffer = ArrayBuffer;
	const checkLength = (what, length, elemSize) => {
		const n = Number(length);
		if (n * elemSize > maxBytes) throw tooLarge(what);
	};
	const checkArrayLike = (what, value, elemSize) => {
		if (typeof value === "number") {
			checkLength(what, value, elemSize);
		} else if (value !== null && typeof value === "object" && !OriginalArrayBuffer.isView(value) && !(value instanceof OriginalArrayBuffer) && typeof value.length === "number") {
			checkLength(what, value.length, elemSize);
		}
	};
	// Each wrapper shares the original prototype, so instanceof, subclassing
	// and inherited statics such as Uint8Array.from keep working, and those
	// statics construct through the wrapper.
	const wrapConstructor = (name) => {
		const original = globalThis[name];
		if (typeof original !== "function") return;
		const elemSize = original.BYTES_PER_ELEMENT || 1;
		const wrapper = {
			[name]: function (...args) {
				if (new.target === undefined) return Reflect.apply(original, this, args);
				checkArrayLike(name, args[0], elemSize);
				return Reflect.construct(original, args, new.target === wrapper ? original : new.target);
			},
		}[name];
		Object.setPrototypeOf(wrapper, Object.getPrototypeOf(original));
		for (const key of Reflect.ownKeys(original)) {
			if (key === "prototype" || key === "length" || key === "name") continue;
			Object.defineProperty(wrapper, key, Object.getOwnPropertyDescriptor(original, key));
		}
		Object.defineProperty(wrapper, "prototype", { value: original.prototype, writable: false, enumerable: false, configurable: false });
		Object.defineProperty(wrapper, "length", { value: original.length, configurable: true });
		Object.defineProperty(original.prototype, "constructor", { value: wrapper, writable: true, enumerable: false, configurable: true });
		Object.defineProperty(globalThis, name, { value: wrapper, writable: true, enumerable: false, configurable: true });
	};
	for (const name of ["ArrayBuffer", "SharedArrayBuffer", "Int8Array", "Uint8Array", "Uint8ClampedArray", "Int16Array", "Uint16Array", "Int32Array", "Uint32Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array"]) {
		wrapConstructor(name);
	}

	const split = String.prototype.split;
	define(String.prototype, "split", function split_(separator, limit) {
		if (this == null || limit !== undefined) return Reflect.apply(split, this, arguments);
		const parts = Reflect.apply(split, this, [separator, maxArrayLength + 1]);
		if (parts.length > maxArrayLength) throw tooLong("String.prototype.split result");
		return parts;
	});

	const checkArrayLength = (what, length) => {
		if (Number(length) > maxArrayLength) throw tooLong(what);
	};
	const OriginalArray = Array;
	const ArrayWrapper = {
		Array: function (...args) {
			if (args.length === 1 && typeof args[0] === "number") checkArrayLength("Array length", args[0]);
			return Reflect.construct(OriginalArray, args, new.target === undefined || new.target === ArrayWrapper ? OriginalArray : new.target);
		},
	}.Array;
	Object.setPrototypeOf(ArrayWrapper, Object.getPrototypeOf(OriginalArray));
	for (const key of Reflect.ownKeys(OriginalArray)) {
		if (key === "prototype" || key === "length" || key === "name") continue;
		Object.defineProperty(ArrayWrapper, key, Object.getOwnPropertyDescriptor(OriginalArray, key));
	}
	Object.defineProperty(ArrayWrapper, "prototype", { value: OriginalArray.prototype, writable: false, enumerable: false, configurable: false });
	Object.defineProperty(ArrayWrapper, "length", { value: OriginalArray.length, configurable: true });
	Object.defineProperty(OriginalArray.prototype, "constructor", { value: ArrayWrapper, writable: true, enumerable: false, configurable: true });
	Object.defineProperty(globalThis, "Array", { value: ArrayWrapper, writable: true, enumerable: false, configurable: true });
	const arrayFrom = OriginalArray.from;
	define(ArrayWrapper, "from", function from(items) {
		if (items != null && typeof items === "object" && typeof items[Symbol.iterator] !== "function") checkArrayLength("Array.from source length", items.length);
		return Reflect.apply(arrayFrom, this === ArrayWrapper ? OriginalArray : this, arguments);
	});
	for (const key of Reflect.ownKeys(OriginalArray.prototype)) {
		const desc = Object.getOwnPropertyDescriptor(OriginalArray.prototype, key);
		if (key === "constructor" || typeof desc.value !== "function") continue;
		const method = desc.value;
		const name = typeof key === "symbol" ? "[" + key.description + "]" : key;
		define(OriginalArray.prototype, key, {
			[name](...args) {
				if (this != null) checkArrayLength("Array.prototype." + name + " receiver length", this.length);
				return Reflect.apply(method, this, args);
			},
		}[name]);
	}

	// join and JSON.stringify run natively, so a single call cannot be
	// interrupted. join is reimplemented as an interruptible loop with a
	// running length check; stringify gets a replacer, which goja calls back
	// into the VM for every value.
	const join = OriginalArray.prototype.join;
	define(OriginalArray.prototype, "join", function join_(separator) {
		const o = Object(this);
		const len = Number(o.length) >>> 0;
		checkArrayLength("Array.prototype.join receiver length", len);
		const sep = separator === undefined ? "," : String(separator);
		let total = len > 0 ? (len - 1) * sep.length : 0;
		if (total > maxBytes) throw tooLarge("Array.prototype.join result");
		const parts = new OriginalArray(len);
		for (let i = 0; i < len; i++) {
			const el = o[i];
			const part = el === undefined || el === null ? "" : String(el);
			total += part.length;
			if (total > maxBytes) throw tooLarge("Array.prototype.join result");
			parts[i] = part;
		}
		return Reflect.apply(join, parts, [sep]);
	});

	const stringify = JSON.stringify;
	define(JSON, "stringify", function stringify_(value, replacer, space) {
		let total = 0;
		const add = (n) => {
			total += n;
			if (total > maxBytes) throw tooLarge("JSON.stringify result");
		};
		let userFn = typeof replacer === "function" ? replacer : undefined;
		if (!userFn && Array.isArray(replacer)) {
			const keys = [];
			for (const k of replacer) {
				if (typeof k === "string" || typeof k === "number" || k instanceof String || k instanceof Number) {
					const key = String(k);
					if (!keys.includes(key)) keys.push(key);
				}
			}
			userFn = function (key, v) {
				if (v === null || typeof v !== "object" || Array.isArray(v)) return v;
				const picked = {};
				for (const k of keys) if (Object.prototype.hasOwnProperty.call(v, k)) picked[k] = v[k];
				return picked;
			};
		}
		const guard = function (key, v) {
			if (userFn) v = Reflect.apply(userFn, this, [key, v]);
			add(String(key).length + 4);
			if (typeof v === "string") add(v.length + 2);
			else if (Array.isArray(v)) checkArrayLength("JSON.stringify array length", v.length);
			return v;
		};
		return Reflect.apply(stringify, this, [value, guard, space]);
	});

	// nestingDepth returns the deepest bracket nesting in text, skipping
	// string literals (and, for source, comments).
	const backtick = String.fromCharCode(96);
	const nestingDepth = (text, source) => {
		let depth = 0;
		for (let i = 0; i < text.length; i++) {
			const ch = text[i];
			if (ch === '"' || (source && (ch === "'" || ch === backtick))) {
				for (i++; i < text.length && text[i] !== ch; i++) if (text[i] === "\\") i++;
			} else if (source && ch === "/" && text[i + 1] === "/") {
				while (i < text.length && text[i] !== "\n") i++;
			} else if (source && ch === "/" && text[i + 1] === "*") {
				const end = text.indexOf("*/", i + 2);
				i = end < 0 ? text.length : end + 1;
			} else if (ch === "(" || ch === "[" || ch === "{") {
				if (++depth > maxNesting) return depth;
			} else if ((ch === ")" || ch === "]" || ch === "}") && depth > 0) {
				depth--;
			}
		}
		return 0;
	};
	const tooDeep = (what) => new RangeError(what + " nesting exceeds the sandbox limit of " + maxNesting + " levels");
	const parse = JSON.parse;
	define(JSON, "parse", function parse_(text, reviver) {
		const s = String(text);
		if (nestingDepth(s, false) > maxNesting) throw tooDeep("JSON.parse input");
		return Reflect.apply(parse, this, [s, reviver]);
	});
	// Replacing eval makes every eval call indirect (global scope). That is
	// the price of checking its source before goja's parser recurses on it.
	const originalEval = globalThis.eval;
	define(globalThis, "eval", {
		eval(source) {
			if (typeof source === "string" && nestingDepth(source, true) > maxNesting) throw tooDeep("eval source");
			return originalEval(source);
		},
	}.eval);
	const OriginalFunction = Function;
	const FunctionWrapper = {
		Function: function (...args) {
			for (const arg of args) if (nestingDepth(String(arg), true) > maxNesting) throw tooDeep("Function source");
			return Reflect.construct(OriginalFunction, args, new.target === undefined || new.target === FunctionWrapper ? OriginalFunction : new.target);
		},
	}.Function;
	Object.setPrototypeOf(FunctionWrapper, Object.getPrototypeOf(OriginalFunction));
	Object.defineProperty(FunctionWrapper, "prototype", { value: OriginalFunction.prototype, writable: false, enumerable: false, configurable: false });
	Object.defineProperty(FunctionWrapper, "length", { value: OriginalFunction.length, configurable: true });
	Object.defineProperty(OriginalFunction.prototype, "constructor", { value: FunctionWrapper, writable: true, enumerable: false, configurable: true });
	Object.defineProperty(globalThis, "Function", { value: FunctionWrapper, writable: true, enumerable: false, configurable: true });

	if (typeof Buffer === "function") {
		const alloc = Buffer.alloc;
		define(Buffer, "alloc", function alloc_(size) {
			checkLength("Buffer.alloc size", size, 1);
			return Reflect.apply(alloc, this, arguments);
		});
		for (const name of ["allocUnsafe", "allocUnsafeSlow"]) {
			if (typeof Buffer[name] !== "function") continue;
			const fn = Buffer[name];
			define(Buffer, name, function (size) {
				checkLength("Buffer." + name + " size", size, 1);
				return Reflect.apply(fn, this, arguments);
			});
		}
		const from = Buffer.from;
		define(Buffer, "from", function from_(value) {
			checkArrayLike("Buffer.from source", typeof value === "number" ? undefined : value, 1);
			return Reflect.apply(from, this, arguments);
		});
		const concat = Buffer.concat;
		define(Buffer, "concat", function concat_(list, totalLength) {
			if (totalLength !== undefined) checkLength("Buffer.concat length", totalLength, 1);
			return Reflect.apply(concat, this, arguments);
		});
	}
})`

// installAllocLimits wraps the allocation-heavy builtins with size checks.
func (rt *nodeRuntime) installAllocLimits() error {
	installer, err := rt.vm.RunString(nodeAllocLimitsScript)
	if err != nil {
		return err
	}
	fn, ok := goja.AssertFunction(installer)
	if !ok {
		return fmt.Errorf("nodejs: allocation limit installer is not a function")
	}
	_, err = fn(goja.Undefined(), rt.vm.ToValue(nodeMaxAllocBytes), rt.vm.ToValue(nodeMaxArrayLength), rt.vm.ToValue(nodeMaxNesting))
	return err
}

// watchExecution interrupts the VM when ctx is done, when the default
// duration expires without a request deadline, or when heap growth exceeds
// nodeMaxHeapGrowthBytes. It returns a stop function.
func (rt *nodeRuntime) watchExecution() func() {
	return watchNodeExecution(rt.ctx, rt.vm, nodeDefaultMaxDuration, nodeMaxHeapGrowthBytes)
}

func watchNodeExecution(ctx context.Context, vm *goja.Runtime, maxDuration time.Duration, maxHeapGrowth uint64) func() {
	done := make(chan struct{})
	go func() {
		var ctxDone <-chan struct{}
		hasDeadline := false
		if ctx != nil {
			ctxDone = ctx.Done()
			_, hasDeadline = ctx.Deadline()
		}
		var timeout <-chan time.Time
		if !hasDeadline && maxDuration > 0 {
			timer := time.NewTimer(maxDuration)
			defer timer.Stop()
			timeout = timer.C
		}
		sample := []metrics.Sample{{Name: nodeHeapMetric}}
		readHeap := func() uint64 {
			metrics.Read(sample)
			if sample[0].Value.Kind() != metrics.KindUint64 {
				return 0
			}
			return sample[0].Value.Uint64()
		}
		heapLimit := readHeap() + maxHeapGrowth
		ticker := time.NewTicker(nodeHeapSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctxDone:
				vm.Interrupt(interruptSignal{err: ctx.Err()})
				return
			case <-timeout:
				vm.Interrupt(interruptSignal{err: errNodeTimeLimit, limit: true})
				return
			case <-ticker.C:
			}
			if readHeap() <= heapLimit {
				continue
			}
			// The metric includes garbage that has not been swept yet;
			// only live growth counts against the budget.
			runtime.GC()
			if readHeap() > heapLimit {
				vm.Interrupt(interruptSignal{err: errNodeMemoryLimit, limit: true})
				return
			}
		}
	}()
	return func() { close(done) }
}

// checkNodeSourceNesting rejects module source (JavaScript or JSON) whose
// bracket nesting exceeds nodeMaxNesting, skipping strings and comments.
func checkNodeSourceNesting(src []byte) error {
	depth := 0
	for i := 0; i < len(src); i++ {
		switch ch := src[i]; {
		case ch == '"' || ch == '\'' || ch == '`':
			for i++; i < len(src) && src[i] != ch; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case ch == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case ch == '/' && i+1 < len(src) && src[i+1] == '*':
			end := bytes.Index(src[i+2:], []byte("*/"))
			if end < 0 {
				return nil
			}
			i += end + 3
		case ch == '(' || ch == '[' || ch == '{':
			depth++
			if depth > nodeMaxNesting {
				return fmt.Errorf("nesting exceeds the sandbox limit of %d levels", nodeMaxNesting)
			}
		case ch == ')' || ch == ']' || ch == '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return nil
}
