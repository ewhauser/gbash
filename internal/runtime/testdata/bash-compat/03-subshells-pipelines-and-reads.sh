#!/usr/bin/env bash
# Exercise pipeline subshell semantics, heredoc reads, and command substitution.

set -euo pipefail

count=0
printf '%s\n' alpha beta gamma | while IFS= read -r _; do
  count=$((count + 1))
done
printf 'after-pipeline:%s\n' "$count"

while IFS= read -r _; do
  count=$((count + 1))
done <<'INNER_READ'
delta
epsilon
INNER_READ
printf 'after-heredoc:%s\n' "$count"

outer=parent
(
  outer=child
  printf 'subshell:%s\n' "$outer"
)
printf 'parent:%s\n' "$outer"

capture=$(printf 'line-1\nline-2\n\n')
printf 'cmdsub-begin\n%s\ncmdsub-end\n' "$capture"
