#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RELEASE_VERSION:-}}"
GOPROXY_URL="${GOPROXY_URL:-https://proxy.golang.org}"
MAX_ATTEMPTS="${PKG_GO_DEV_MAX_ATTEMPTS:-20}"
SLEEP_SECONDS="${PKG_GO_DEV_RETRY_DELAY_SECONDS:-15}"

if [[ -z "${VERSION}" ]]; then
	echo "usage: scripts/publish_pkgsite.sh vX.Y.Z" >&2
	exit 1
fi

if [[ "${VERSION}" != v* ]]; then
	echo "version must look like vX.Y.Z" >&2
	exit 1
fi

if ! command -v go >/dev/null 2>&1; then
	echo "go toolchain is required" >&2
	exit 1
fi

module_path_from_go_mod() {
	local file="$1"
	local module_path

	module_path="$(sed -n 's/^module //p' "${file}" | head -n 1)"
	if [[ -z "${module_path}" ]]; then
		echo "unable to determine module path from ${file}" >&2
		exit 1
	fi

	printf '%s\n' "${module_path}"
}

modules=()
modules+=("$(module_path_from_go_mod "${ROOT_DIR}/go.mod")")
while IFS= read -r go_mod; do
	modules+=("$(module_path_from_go_mod "${ROOT_DIR}/${go_mod}")")
done < <(
	cd "${ROOT_DIR}"
	find contrib -mindepth 2 -maxdepth 2 -type f -name go.mod | sort
)

temp_dir="$(mktemp -d)"
cleanup() {
	rm -rf "${temp_dir}"
}
trap cleanup EXIT

printf 'requesting pkg.go.dev indexing for %s via %s\n' "${VERSION}" "${GOPROXY_URL}"
for module in "${modules[@]}"; do
	module_version="${module}@${VERSION}"
	for ((attempt = 1; attempt <= MAX_ATTEMPTS; attempt++)); do
		if output="$(
			cd "${temp_dir}"
			GOWORK=off GO111MODULE=on GOPROXY="${GOPROXY_URL}" go list -m "${module_version}" 2>&1
		)"; then
			printf '  published %s\n' "${module_version}"
			break
		fi

		if ((attempt == MAX_ATTEMPTS)); then
			printf 'failed to publish %s after %d attempts\n%s\n' "${module_version}" "${MAX_ATTEMPTS}" "${output}" >&2
			exit 1
		fi

		printf '  waiting for proxy to serve %s (attempt %d/%d)\n' "${module_version}" "${attempt}" "${MAX_ATTEMPTS}"
		sleep "${SLEEP_SECONDS}"
	done
done
