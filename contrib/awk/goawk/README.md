This directory contains the minimal local `goawk` fork used by `contrib/awk`.

- Upstream: `github.com/benhoyt/goawk`
- Forked from: `v1.31.0`
- Local change: `interp.Config` accepts a gbash-controlled file opener so awk input files and `getline <file>` can read sandbox-backed files without falling through to host `os.Open`.
- Local change: `interp.Config.MaxStringBytes` and `MaxArrayBytes` bound the strings and arrays a program can build (see `interp/limits.go`), so untrusted scripts fail with an error instead of growing host memory without bound.

`LICENSE.upstream.txt` preserves the upstream license text.
