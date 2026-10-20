# Changelog

## [0.0.41](https://github.com/ewhauser/gbash/compare/v0.0.40...v0.0.41) (2026-10-20)


### Bug Fixes

* **builtins:** bound file copies, archive output, and backup paths ([#887](https://github.com/ewhauser/gbash/issues/887)) ([529fd30](https://github.com/ewhauser/gbash/commit/529fd3064a2db1329cff60687e40660b2852c2c5))
* **builtins:** harden xan, factor, curl, rev, sed, seq, uptime and who ([#889](https://github.com/ewhauser/gbash/issues/889)) ([2fdc2f0](https://github.com/ewhauser/gbash/commit/2fdc2f01d341ba66ca03a429737d75519e1b8c13))
* **contrib:** bound nesting, sizes and REPL input across contrib modules ([#891](https://github.com/ewhauser/gbash/issues/891)) ([0f37e75](https://github.com/ewhauser/gbash/commit/0f37e75b89bec604f5865d81a6319af80232a622))
* **expand:** fix nested arithmetic blowup, @ operator panic, and pattern matcher bugs ([#892](https://github.com/ewhauser/gbash/issues/892)) ([a504362](https://github.com/ewhauser/gbash/commit/a50436226ed6557299448d930ba5bb4bd68ef4f1))
* **fs:** harden file backends from second review ([#890](https://github.com/ewhauser/gbash/issues/890)) ([fe39231](https://github.com/ewhauser/gbash/commit/fe39231dd463c63a41a5b19a06aab19f96de0590))
* **interp:** make deep function recursion linear in its depth ([#883](https://github.com/ewhauser/gbash/issues/883)) ([25d4d05](https://github.com/ewhauser/gbash/commit/25d4d0574a0e4e44fdd2453f4454876cace0fa18))
* **network:** redact secrets in errors, traces and cross-origin redirects ([#886](https://github.com/ewhauser/gbash/issues/886)) ([b34bc0c](https://github.com/ewhauser/gbash/commit/b34bc0c4e20d999192ac9fd5e3d7356586d1fa3a))
* **server:** scope sessions to connections and harden server limits ([#888](https://github.com/ewhauser/gbash/issues/888)) ([b462aa0](https://github.com/ewhauser/gbash/commit/b462aa09a0d1eb8664ecd30ea7f0c9d317c07bd6))
* **shell:** close loop, eval, background-job and glob budget bypasses ([#893](https://github.com/ewhauser/gbash/issues/893)) ([57be482](https://github.com/ewhauser/gbash/commit/57be482b7f033f794d9d89d4c88326c8e0aa945b))
* **syntax:** cap pattern nesting through shell parts ([#879](https://github.com/ewhauser/gbash/issues/879)) ([f0fdaba](https://github.com/ewhauser/gbash/commit/f0fdaba6af5a88e3e34ae421c49bbe5051f2f526))
* **syntax:** keep chunked parses intact and nested paren probes fast ([#894](https://github.com/ewhauser/gbash/issues/894)) ([cffed2a](https://github.com/ewhauser/gbash/commit/cffed2aab35421088d62e563fbd7d026f026b98e))
* **syntax:** parse case patterns that run past the 1 KiB read buffer ([#880](https://github.com/ewhauser/gbash/issues/880)) ([bfd6646](https://github.com/ewhauser/gbash/commit/bfd664602552594874b59c2cbc9e2614b77ff234))
* **syntax:** stop nested patterns from parsing in exponential time ([#877](https://github.com/ewhauser/gbash/issues/877)) ([43326a9](https://github.com/ewhauser/gbash/commit/43326a929368f467264a11a6290728bdee4984e6))
* **syntax:** stop slow parses and hangs around heredoc bodies ([#885](https://github.com/ewhauser/gbash/issues/885)) ([19ff56a](https://github.com/ewhauser/gbash/commit/19ff56af264701ab3b50994d70fbda538c941266))

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
