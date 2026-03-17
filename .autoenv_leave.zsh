if [ -n "$_AUTOENV_SNAPSHOT" ] && [ -f "$_AUTOENV_SNAPSHOT" ]; then
  # Unset variables added by nix that were not in the original environment
  _old_vars=$(grep -oE '^export [A-Za-z_][A-Za-z_0-9]*=' "$_AUTOENV_SNAPSHOT" | sed 's/^export //;s/=$//' | sort)
  _cur_vars=$(typeset -px | grep -oE '^export [A-Za-z_][A-Za-z_0-9]*=' | sed 's/^export //;s/=$//' | sort)
  for _v in $(comm -23 <(echo "$_cur_vars") <(echo "$_old_vars")); do
    unset "$_v" 2>/dev/null
  done

  # Restore original variable values
  source "$_AUTOENV_SNAPSHOT"

  rm -f "$_AUTOENV_SNAPSHOT"
  unset _AUTOENV_SNAPSHOT _old_vars _cur_vars _v
fi
