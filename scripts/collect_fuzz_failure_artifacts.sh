#!/usr/bin/env bash

set -euo pipefail

# The directory is removed and recreated below, so never fall back to a path
# rooted at / when neither an argument nor RUNNER_TEMP is set.
if [ -n "${1:-}" ]; then
  artifact_dir="$1"
else
  artifact_dir="${RUNNER_TEMP:?artifact directory argument or RUNNER_TEMP is required}/fuzz-failure-artifacts"
fi
case "${artifact_dir}" in
  "" | / | // | /. | /..)
    echo "refusing to use artifact directory: ${artifact_dir}" >&2
    exit 1
    ;;
esac

output_file="${GITHUB_OUTPUT:-}"
summary_file="${GITHUB_STEP_SUMMARY:-}"

rm -rf "${artifact_dir}"
mkdir -p "${artifact_dir}/files" "${artifact_dir}/base64"

manifest_file="${artifact_dir}/manifest.md"
{
  echo "# Fuzz Failure Inputs"
  echo
} > "${manifest_file}"

if [ -n "${summary_file}" ]; then
  {
    echo "## Fuzz Failure Inputs"
    echo
  } >> "${summary_file}"
fi

files=()
while IFS= read -r path; do
  [ -n "${path}" ] || continue

  rel_path="${path#./}"
  if git ls-files --error-unmatch -- "${rel_path}" >/dev/null 2>&1; then
    if [ -z "$(git status --short -- "${rel_path}")" ]; then
      continue
    fi
  fi

  files+=("${path}")
done < <(find . -type f -path '*/testdata/fuzz/Fuzz*/*' | LC_ALL=C sort)

if [ "${#files[@]}" -eq 0 ]; then
  {
    echo "No new or modified fuzz corpus files were detected after the failing run."
    echo
  } >> "${manifest_file}"
  if [ -n "${summary_file}" ]; then
    {
      echo "No new or modified fuzz corpus files were detected after the failing run."
      echo
    } >> "${summary_file}"
  fi
  if [ -n "${output_file}" ]; then
    {
      echo "has_files=false"
      echo "artifact_dir=${artifact_dir}"
    } >> "${output_file}"
  fi
  exit 0
fi

count=0
for path in "${files[@]}"; do
  [ -f "${path}" ] || continue

  rel_path="${path#./}"
  dest_path="${artifact_dir}/files/${rel_path}"
  mkdir -p "$(dirname "${dest_path}")"
  cp "${path}" "${dest_path}"

  encoded_path="${artifact_dir}/base64/${rel_path}.b64"
  mkdir -p "$(dirname "${encoded_path}")"
  base64 < "${path}" > "${encoded_path}"

  pkg_path="${rel_path%%/testdata/fuzz/*}"
  fuzz_suffix="${rel_path#${pkg_path}/testdata/fuzz/}"
  fuzz_target="${fuzz_suffix%%/*}"
  corpus_name="${fuzz_suffix#${fuzz_target}/}"
  rerun_cmd="go test ./${pkg_path} -run='${fuzz_target}/${corpus_name}'"
  size_bytes="$(wc -c < "${path}" | tr -d '[:space:]')"
  sha256="$(shasum -a 256 "${path}" | awk '{print $1}')"

  {
    echo "Path: \`${rel_path}\`"
    echo "Re-run: \`${rerun_cmd}\`"
    echo "Bytes: ${size_bytes}"
    echo "SHA256: \`${sha256}\`"
    echo "Base64 Copy: \`base64/${rel_path}.b64\`"
    echo
  } >> "${manifest_file}"

  if [ -n "${summary_file}" ]; then
    {
      echo "Path: \`${rel_path}\`"
      echo "Re-run: \`${rerun_cmd}\`"
      echo "Bytes: ${size_bytes}"
      echo "SHA256: \`${sha256}\`"
      echo
    } >> "${summary_file}"
  fi

  count=$((count + 1))
done

if [ -n "${output_file}" ]; then
  {
    echo "has_files=true"
    echo "artifact_dir=${artifact_dir}"
    echo "count=${count}"
  } >> "${output_file}"
fi
