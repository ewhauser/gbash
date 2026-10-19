package nodejs

import (
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

	nodeHeapSampleInterval = 10 * time.Millisecond
	nodeHeapMetric         = "/memory/classes/heap/objects:bytes"
)

var (
	errNodeTimeLimit   = fmt.Errorf("execution exceeded the %s time limit", nodeDefaultMaxDuration)
	errNodeMemoryLimit = fmt.Errorf("execution exceeded the %d MiB memory limit", nodeMaxHeapGrowthBytes>>20)
)

// nodeAllocLimitsScript wraps the builtins that allocate a caller-chosen
// size up front or loop natively over a caller-chosen length.
const nodeAllocLimitsScript = `(function (maxBytes, maxArrayLength) {
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
	_, err = fn(goja.Undefined(), rt.vm.ToValue(nodeMaxAllocBytes), rt.vm.ToValue(nodeMaxArrayLength))
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
