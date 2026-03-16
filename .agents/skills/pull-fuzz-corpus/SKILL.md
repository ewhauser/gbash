---
name: pull-fuzz-corpus
description: Pull failing fuzz test corpus files from GitHub Actions CI and copy them into the project as regression tests. Use this skill whenever the user mentions pulling, downloading, or importing fuzz failures from CI, or pastes a link to a failed fuzz GitHub Actions run, or asks about fuzz corpus files from CI.
---

# Pull Fuzz Corpus from CI

Run the script at `scripts/pull_fuzz_corpus.py` (relative to this skill's directory) to find and download failing fuzz corpus files from CI.

## Usage

```bash
# Find and pull all new fuzz failures (skips already-processed runs)
python <skill-dir>/scripts/pull_fuzz_corpus.py

# Pull from a specific run ID or URL
python <skill-dir>/scripts/pull_fuzz_corpus.py --run-id <run_id>

# See what would be pulled without downloading
python <skill-dir>/scripts/pull_fuzz_corpus.py --dry-run

# Re-check all failed runs, ignoring the watermark
python <skill-dir>/scripts/pull_fuzz_corpus.py --all

# Reset the watermark to start fresh
python <skill-dir>/scripts/pull_fuzz_corpus.py --reset-watermark
```

If the user provides a GitHub Actions URL, extract the run ID from it (the numeric segment after `/runs/`).

## What the script does

1. Queries `gh` for recent failed runs across the `fuzz-full.yml` and `ci.yml` workflows
2. Checks a high watermark file (`.fuzz-corpus-watermark.json` at the repo root) to skip runs that have already been processed
3. For each new failed run, finds the failed fuzz jobs and parses their logs for `Failing input written to` lines
4. Downloads the corresponding corpus artifacts and copies them into the correct `testdata/fuzz/` directories
5. Updates the watermark

## After pulling

After the script copies files, run the failing test to verify it reproduces locally. The script prints the exact `go test -run=...` command to use. Report the result to the user.
