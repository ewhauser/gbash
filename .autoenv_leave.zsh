if [ -n "$_AUTOENV_OLD_PATH" ]; then
  export PATH="$_AUTOENV_OLD_PATH"
  unset _AUTOENV_OLD_PATH
fi
