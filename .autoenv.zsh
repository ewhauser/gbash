if command -v nix >/dev/null 2>&1; then
  _AUTOENV_SNAPSHOT=$(mktemp "${TMPDIR:-/tmp}/autoenv.XXXXXX")
  export _AUTOENV_SNAPSHOT
  typeset -px > "$_AUTOENV_SNAPSHOT"
  eval "$(nix print-dev-env)"
fi
