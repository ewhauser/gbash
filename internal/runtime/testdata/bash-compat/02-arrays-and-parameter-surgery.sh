#!/usr/bin/env bash
# Exercise indexed arrays, slices, replacement, and indirect expansion.

set -euo pipefail

arr=(zero one "two words" three)
index=2
word='bananarama'
ref=word

printf 'len:%s\n' "${#arr[@]}"
printf 'slice:%s|%s\n' "${arr[@]:1:2}"
printf 'picked:%s\n' "${arr[$index]}"
printf 'replace:%s\n' "${word//a/A}"
printf 'indirect:%s\n' "${!ref}"

for idx in "${!arr[@]}"; do
  printf 'arr[%s]=<%s>\n' "$idx" "${arr[$idx]}"
done
