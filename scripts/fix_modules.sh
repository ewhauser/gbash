#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${VERSION:-}}"

if [[ -n "${VERSION}" && "${VERSION}" != v* ]]; then
	echo "version must look like vX.Y.Z" >&2
	exit 1
fi

sync_package_json_version() {
	local file="$1"
	if [[ -z "${VERSION}" || ! -f "${ROOT_DIR}/${file}" ]]; then
		return
	fi
	node - "${ROOT_DIR}/${file}" "${VERSION#v}" <<'EOF'
const fs = require("node:fs");

const [, , file, version] = process.argv;
const pkg = JSON.parse(fs.readFileSync(file, "utf8"));
pkg.version = version;
fs.writeFileSync(file, `${JSON.stringify(pkg, null, 2)}\n`);
EOF
}

edit_module() {
	local dir="$1"
	shift
	(
		cd "${ROOT_DIR}/${dir}"
		while (($#)); do
			local module="$1"
			if [[ -n "${VERSION}" ]]; then
				go mod edit -require="${module}@${VERSION}"
			fi
			shift
		done
		go mod tidy
	)
}

edit_module contrib/awk github.com/ewhauser/gbash
edit_module contrib/codingtools github.com/ewhauser/gbash
edit_module contrib/htmltomarkdown github.com/ewhauser/gbash
edit_module contrib/jq github.com/ewhauser/gbash
edit_module contrib/nodejs github.com/ewhauser/gbash
edit_module contrib/sqlite3 github.com/ewhauser/gbash
edit_module contrib/yq github.com/ewhauser/gbash
edit_module \
	contrib/extras \
	github.com/ewhauser/gbash \
	github.com/ewhauser/gbash/contrib/awk \
	github.com/ewhauser/gbash/contrib/htmltomarkdown \
	github.com/ewhauser/gbash/contrib/jq \
	github.com/ewhauser/gbash/contrib/sqlite3 \
	github.com/ewhauser/gbash/contrib/yq
edit_module \
	contrib/bashtool \
	github.com/ewhauser/gbash \
	github.com/ewhauser/gbash/contrib/extras
edit_module \
	examples \
	github.com/ewhauser/gbash \
	github.com/ewhauser/gbash/contrib/bashtool \
	github.com/ewhauser/gbash/contrib/extras \
	github.com/ewhauser/gbash/contrib/sqlite3

sync_package_json_version packages/gbash-wasm/package.json

(
	cd "${ROOT_DIR}"
	go list -m all > /dev/null
)
