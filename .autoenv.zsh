if command -v nix >/dev/null 2>&1; then
  export _AUTOENV_OLD_PATH="$PATH"
  eval "$(nix print-dev-env)"
fi
