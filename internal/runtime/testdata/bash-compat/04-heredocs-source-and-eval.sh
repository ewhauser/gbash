#!/usr/bin/env bash
# Exercise quoted vs unquoted heredocs, source, read splitting, and eval.

set -euo pipefail

prefix=spot

cat > helper.sh <<'HELPER'
helper_name=from-helper
emit_helper() {
  printf 'helper:%s\n' "$helper_name"
}
HELPER

. ./helper.sh
emit_helper

read first rest <<'READ_SPLIT'
alpha   beta gamma
READ_SPLIT
printf 'read:%s|%s\n' "$first" "$rest"

cat <<EXPAND
expand:$prefix:$((2 + 3))
EXPAND

cat <<'LITERAL'
literal:$prefix:$((2 + 3))
LITERAL

target=prefix
eval "$target=\${$target}:mutated"
printf 'eval:%s\n' "$prefix"
