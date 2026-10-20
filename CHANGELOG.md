# Changelog

## [0.0.40](https://github.com/ewhauser/gbash/compare/v0.0.39...v0.0.40) (2026-10-20)


### Bug Fixes

* **builtins:** bound memory and runtime in text-processing builtins ([#872](https://github.com/ewhauser/gbash/issues/872)) ([ed58d4e](https://github.com/ewhauser/gbash/commit/ed58d4e133869bec517b862ec17b517ecbc4b242))
* **builtins:** harden file builtins against host probes, size bombs and symlink escapes ([#874](https://github.com/ewhauser/gbash/issues/874)) ([3f35f21](https://github.com/ewhauser/gbash/commit/3f35f21671ed22e46e65f486a5c5a27c0dc2b496))
* **contrib:** bound memory and run time of embedded interpreters ([#873](https://github.com/ewhauser/gbash/issues/873)) ([3da5d0a](https://github.com/ewhauser/gbash/commit/3da5d0a0d67659a1b12f66d99cfd2f325c175bbf))
* **fs:** fix FIFO hangs, overlay copy-up, and handle and mode bugs ([#870](https://github.com/ewhauser/gbash/issues/870)) ([ed5f677](https://github.com/ewhauser/gbash/commit/ed5f677ad3ede858c64386a5ab25bf2d92ba3b53))
* harden redirects, trace redaction, and custom-policy limits ([#869](https://github.com/ewhauser/gbash/issues/869)) ([9d5463c](https://github.com/ewhauser/gbash/commit/9d5463c4e9d5abf10e5cbbb2336254e5753ee9b4))
* **interp:** bound shell nesting, budgets, and variable memory ([#875](https://github.com/ewhauser/gbash/issues/875)) ([5c55c14](https://github.com/ewhauser/gbash/commit/5c55c14a97137e3ff899b1a8e258922d9ec5ebd6))
* **server:** harden server mode, CLI flag parsing, and readwrite-root mounting ([#871](https://github.com/ewhauser/gbash/issues/871)) ([dc9e28e](https://github.com/ewhauser/gbash/commit/dc9e28e0b74d1f8a28c9fd0c64245e888fb920b4))
* **syntax:** stop crashes and hangs from deep or malformed shell input ([#876](https://github.com/ewhauser/gbash/issues/876)) ([3e89dee](https://github.com/ewhauser/gbash/commit/3e89deef12f4d807c5716093562bc4158e5a321c))
