#### stdin yaml eval
printf 'name: alice\nteam: core\n' | yq '.name'

#### json file auto detection
printf '%s\n' '{"name":"alice","team":"core"}' >input.json
yq '.name' input.json

#### null input creation
yq -n '.a.b = "cat"'

#### expression from file
printf '%s\n' '.team' >filter.yq
printf 'name: alice\nteam: core\n' | yq --from-file filter.yq

#### eval all across files
printf 'name: alice\n' >a.yml
printf 'team: core\n' >b.yml
yq ea 'select(fileIndex == 0) * select(fileIndex == 1)' a.yml b.yml

#### output formatting flags
printf '%s\n' '{"a":1,"b":2}' | yq -p json -o json -I 0 '.'

#### wrapped scalar output
printf 'name: alice\n' | yq -o json --unwrapScalar=false '.name'

#### nul separated output
printf '%s\n' '- a' '- b' | yq -0 '.[]' | od -An -t x1 | tr -d ' \n'
printf '\n'

#### exit status when no matches found
printf 'name: alice\n' >input.yml
set +e
yq -e '.missing' input.yml >stdout.txt 2>stderr.txt
rc=$?
set -e
printf 'rc=%s\n' "$rc"
od -An -t x1 stdout.txt | tr -d ' \n'
printf '\n'
case "$(cat stderr.txt)" in
  *"no matches found"*) printf 'stderr\n' ;;
  *) printf 'missing-stderr\n' ;;
esac

#### in place edit
printf 'name: alice\n' >doc.yml
yq -i '.name = "bob"' doc.yml
cat doc.yml

#### invalid expression failure
set +e
stderr="$(yq -n '.foo[' 2>&1 >/dev/null)"
rc=$?
set -e
printf 'rc=%s\n' "$rc"
case "$stderr" in
  *"bad expression"*) printf 'parse-error\n' ;;
  *) printf 'missing-parse-error\n' ;;
esac

#### missing input file failure
set +e
stderr="$(yq '.name' missing.yml 2>&1 >/dev/null)"
rc=$?
set -e
printf 'rc=%s\n' "$rc"
case "$stderr" in
  *"missing.yml"*) printf 'missing-file\n' ;;
  *) printf 'missing-filename\n' ;;
esac

#### missing expression file failure
set +e
stderr="$(yq --from-file missing.yq 2>&1 >/dev/null)"
rc=$?
set -e
printf 'rc=%s\n' "$rc"
case "$stderr" in
  *"missing.yq"*) printf 'missing-expression-file\n' ;;
  *) printf 'missing-expression-filename\n' ;;
esac

#### load operator denied by sandbox
printf 'team: core\n' >other.yml
yq -n 'load("other.yml")'

#### load str operator denied by sandbox
printf 'hello\n' >message.txt
yq -n 'load_str("message.txt")'

#### env operator denied by sandbox
MY_VAR='{"team":"core"}' yq -n 'env(MY_VAR).team'

#### strenv operator denied by sandbox
MY_VAR=secret yq -n 'strenv(MY_VAR)'

#### envsubst operator denied by sandbox
MY_VAR=secret yq -n '"value: ${MY_VAR}" | envsubst'
