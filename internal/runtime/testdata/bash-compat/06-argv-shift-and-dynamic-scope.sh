#!/usr/bin/env bash
# Exercise positional parameters, shift, quoting, and dynamic scope via locals.

set -euo pipefail

set -- alpha 'beta gamma' delta

printf 'argc:%s\n' "$#"
printf 'quoted-args-begin\n'
printf '<%s>\n' "$@"
printf 'quoted-args-end\n'

shift
printf 'argc-after-shift:%s\n' "$#"

join_with() {
  local sep=$1
  shift
  local out=
  local item

  for item in "$@"; do
    if [ -n "$out" ]; then
      out="${out}${sep}"
    fi
    out="${out}${item}"
  done

  printf 'joined:%s\n' "$out"
}

outer() {
  local state=outer
  inner() {
    printf 'inner-before:%s\n' "$state"
    local state=inner
    printf 'inner-after:%s\n' "$state"
  }

  inner
  printf 'outer-after:%s\n' "$state"
}

join_with ':' "$@"
outer
