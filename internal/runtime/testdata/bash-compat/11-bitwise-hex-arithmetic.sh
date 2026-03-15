#!/usr/bin/env bash
# Exercise arithmetic with hex literals, bitwise operators, pre/post inc/dec,
# ternary, and the comma operator.

set -euo pipefail

x=5
printf 'ternary:%s\n' "$(( x > 3 ? 10 : 20 ))"
printf 'pre-inc:%s\n' "$(( ++x ))"
printf 'post-dec:%s\n' "$(( x-- ))"
printf 'after-dec:%s\n' "$x"
printf 'bitwise:%s\n' "$(( (0xFF & 0x0F) | 0x30 ))"
printf 'comma:%s\n' "$(( x=100, x/4 ))"
