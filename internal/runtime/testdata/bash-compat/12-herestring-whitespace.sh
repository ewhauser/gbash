#!/usr/bin/env bash
# Exercise here-string IFS whitespace trimming via read.

set -euo pipefail

read -r val <<<"   padded   "
printf 'herestr:<%s>\n' "$val"
read -r a b c <<<"one two three"
printf 'herestr-split:%s|%s|%s\n' "$a" "$b" "$c"
