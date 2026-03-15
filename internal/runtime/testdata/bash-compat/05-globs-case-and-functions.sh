#!/usr/bin/env bash
# Exercise brace expansion, globs, case dispatch, and local variables.

set -euo pipefail

mkdir -p tree/one tree/two
: > tree/one/a.txt
: > tree/one/b.log
: > tree/two/c.txt

emit_tags() {
  local path dir base tag

  for path in tree/*/*.{txt,log}; do
    dir=${path%/*}
    base=${path##*/}
    case "$base" in
      *.txt) tag=text ;;
      *.log) tag=log ;;
      *) tag=other ;;
    esac
    printf '%s -> %s/%s:%s\n' "$path" "$dir" "$tag" "$base"
  done
}

emit_tags
