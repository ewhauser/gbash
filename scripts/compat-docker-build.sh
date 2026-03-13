#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

IMAGE_NAME=${COMPAT_DOCKER_IMAGE:-gbash-compat-local}
PLATFORM=${COMPAT_DOCKER_PLATFORM:-linux/amd64}
GO_VERSION=${COMPAT_DOCKER_GO_VERSION:-$(awk '/^go / { print $2; exit }' "$REPO_ROOT/go.mod")}

exec docker build \
  --platform "$PLATFORM" \
  --build-arg "GO_VERSION=$GO_VERSION" \
  -t "$IMAGE_NAME" \
  -f "$REPO_ROOT/docker/compat/Dockerfile" \
  "$REPO_ROOT"
