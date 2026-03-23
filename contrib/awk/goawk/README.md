This directory contains the minimal local `goawk` fork used by `contrib/awk`.

- Upstream: `github.com/benhoyt/goawk`
- Forked from: `v1.31.0`
- Local change: `interp.Config` accepts a gbash-controlled file opener so awk input files and `getline <file>` can read sandbox-backed files without falling through to host `os.Open`.

`LICENSE.upstream.txt` preserves the upstream license text.
