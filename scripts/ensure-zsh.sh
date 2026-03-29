#!/usr/bin/env bash
set -euo pipefail

# If GBASH_CONFORMANCE_ZSH is already set, use it directly
if [[ -n "${GBASH_CONFORMANCE_ZSH:-}" ]]; then
  echo "$GBASH_CONFORMANCE_ZSH"
  exit 0
fi

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

if ! command -v nix >/dev/null 2>&1; then
  echo "error: nix is not installed" >&2
  echo "" >&2
  echo "Install Nix to get the pinned zsh binary for conformance tests:" >&2
  echo "" >&2
  echo "  macOS:  sh <(curl -L https://nixos.org/nix/install)" >&2
  echo "  Linux:  sh <(curl -L https://nixos.org/nix/install) --daemon" >&2
  echo "" >&2
  echo "After installation, restart your shell and re-run this command." >&2
  echo "" >&2
  echo "Alternatively, set GBASH_CONFORMANCE_ZSH to skip Nix:" >&2
  echo "  export GBASH_CONFORMANCE_ZSH=/path/to/zsh" >&2
  exit 1
fi

zsh_path=""
for out in $(nix build "${REPO_ROOT}#zsh" --no-link --print-out-paths --extra-experimental-features 'nix-command flakes' 2>/dev/null); do
  if [[ -x "${out}/bin/zsh" ]]; then
    zsh_path="${out}/bin/zsh"
    break
  fi
done

if [[ -z "$zsh_path" ]]; then
  echo "error: nix build failed to produce zsh binary" >&2
  exit 1
fi

echo "$zsh_path"
