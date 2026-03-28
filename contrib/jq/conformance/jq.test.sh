#### stdin raw output
printf '%s\n' '{"name":"test"}' | jq -r '.name'

#### compact output across multiple files
printf '%s\n' '{"id":1}' >a.json
printf '%s\n' '{"id":2}' >b.json
jq -c '.' a.json b.json

#### slurp stdin values
printf '1\n2\n3\n' | jq -s '.'

#### null input empty output
jq -n 'empty'

#### stdin marker with file
printf '%s\n' '{"from":"file"}' >file.json
printf '%s\n' '{"from":"stdin"}' | jq -r '.from' - file.json

#### raw input file
printf 'alpha\nbeta\n' >in.txt
jq -R '.' in.txt

#### filter from file
printf '%s\n' '.name' >filter.jq
printf '%s\n' '{"name":"alice"}' | jq -r -f filter.jq

#### arg and argjson
jq -n -c --arg name alice --argjson meta '{"team":"core"}' '{name: $name, team: $meta.team}'

#### slurpfile and rawfile
printf '1\n2\n3\n' >nums.json
printf 'hello\n' >message.txt
jq -n -c --slurpfile nums nums.json --rawfile msg message.txt '{count: ($nums | length), msg: $msg}'

#### args and jsonargs
jq -n '$ARGS.positional[1]' --args one two
jq -n '$ARGS.positional[1].x' --jsonargs '1' '{"x":2}'

#### raw output zero delimiter
printf '%s\n' '["a","b"]' | jq -r --raw-output0 '.[]' | od -An -t x1 | tr -d ' \n'
printf '\n'

#### indent formatting
printf '%s\n' '{"a":1}' | jq --indent 4 '.'

#### tab formatting
printf '%s\n' '{"a":1}' | jq --tab '.'

#### exit status tracks false output
printf '%s\n' 'false' | jq -e '.'
printf 'exit=%s\n' "$?"

#### invalid json failure
set +e
stderr="$(printf '%s\n' 'not json' | jq '.' 2>&1 >/dev/null)"
rc=$?
set -e
printf 'rc=%s\n' "$rc"
case "$stderr" in
  *"parse error"*) printf 'parse-error\n' ;;
  *) printf 'missing-parse-error\n' ;;
esac

#### invalid query failure
set +e
stderr="$(jq 'if . then' </dev/null 2>&1 >/dev/null)"
rc=$?
set -e
printf 'rc=%s\n' "$rc"
case "$stderr" in
  "") printf 'missing-stderr\n' ;;
  *) printf 'stderr\n' ;;
esac

#### missing file failure
set +e
stderr="$(jq '.x' missing.json 2>&1 >/dev/null)"
rc=$?
set -e
printf 'rc=%s\n' "$rc"
case "$stderr" in
  *"missing.json"*) printf 'missing-file\n' ;;
  *) printf 'missing-filename\n' ;;
esac

#### default output preserves object key order
printf '%s\n' '{"z":0,"a":1}' | jq -c '.'

#### ascii output escapes unicode
printf '%s\n' '{"x":"Ω"}' | jq --ascii-output -c '.'

#### version short flag
set +e
out="$(jq -V)"
rc=$?
set -e
printf 'rc=%s\n' "$rc"
case "$out" in
  "") printf 'empty\n' ;;
  *) printf 'non-empty\n' ;;
esac

#### inputs builtin with null input
printf '%s\n' '1' '2' | jq -n '[inputs]'

#### input filename builtin
printf '%s\n' '1' >a.json
jq 'input_filename' a.json

#### unbuffered output flag
jq --unbuffered -n '1'

#### stream mode
printf '%s\n' '[1,[2]]' | jq --stream '.'

#### stream errors mode
printf '%s' '{' | jq --stream-errors '.'

#### json seq mode
printf '%s\n' '1' | jq --seq '.'

#### library path include
mkdir -p lib
printf '%s\n' 'def f: 42;' >lib/mod.jq
jq -L lib -n 'include "mod"; f'

#### color output long flag
out="$(printf '%s\n' '{"x":1}' | jq --color-output -c '.')"
case "$out" in
  *$'\033'*) printf 'ansi\n' ;;
  *) printf 'plain\n' ;;
esac

#### monochrome output long flag
printf '%s\n' '{"x":1}' | jq --monochrome-output -c '.'

#### build configuration flag
set +e
out="$(jq --build-configuration)"
rc=$?
set -e
printf 'rc=%s\n' "$rc"
case "$out" in
  "") printf 'empty\n' ;;
  *) printf 'non-empty\n' ;;
esac
