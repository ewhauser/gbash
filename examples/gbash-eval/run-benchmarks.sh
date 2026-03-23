#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/../.." && pwd)"

dataset="${GBASH_EVAL_DATASET:-${script_dir}/data/eval-tasks.jsonl}"
output_dir="${GBASH_EVAL_OUTPUT_DIR:-${script_dir}/results}"
moniker_prefix="${GBASH_EVAL_MONIKER_PREFIX:-benchmark}"

# provider|model|required_api_key
run_matrix=(
  "anthropic|${GBASH_EVAL_ANTHROPIC_MODEL:-claude-opus-4-6}|ANTHROPIC_API_KEY"
  "${GBASH_EVAL_OPENAI_PROVIDER:-openai}|${GBASH_EVAL_OPENAI_MODEL:-gpt-5.4}|OPENAI_API_KEY"
)
selected_run_matrix=()

if [[ ! -f "${dataset}" ]]; then
  echo "Dataset not found: ${dataset}" >&2
  exit 1
fi

mkdir -p "${output_dir}"

moniker_for() {
  local value="$1"
  value="${value//\//-}"
  value="${value//:/-}"
  value="${value// /-}"
  value="${value//$'\t'/-}"
  printf '%s' "${value}"
}

print_run_matrix() {
  echo "Configured benchmark matrix:"
  printf '  %-14s %-28s %s\n' "provider" "model" "required_api_key"
  local entry provider model required_api_key
  for entry in "${run_matrix[@]}"; do
    IFS='|' read -r provider model required_api_key <<<"${entry}"
    printf '  %-14s %-28s %s\n' "${provider}" "${model}" "${required_api_key}"
  done
}

required_api_key_for_provider() {
  local provider="$1"
  case "${provider}" in
    anthropic)
      printf '%s' "ANTHROPIC_API_KEY"
      ;;
    openai|openresponses)
      printf '%s' "OPENAI_API_KEY"
      ;;
    *)
      echo "Unsupported provider: ${provider}" >&2
      return 1
      ;;
  esac
}

default_model_for_provider() {
  local provider="$1"
  local entry current_provider model required_api_key
  for entry in "${run_matrix[@]}"; do
    IFS='|' read -r current_provider model required_api_key <<<"${entry}"
    if [[ "${current_provider}" == "${provider}" ]]; then
      printf '%s' "${model}"
      return 0
    fi
  done
  case "${provider}" in
    anthropic)
      printf '%s' "claude-opus-4-6"
      ;;
    openai|openresponses)
      printf '%s' "gpt-5.4"
      ;;
    *)
      echo "Unsupported provider: ${provider}" >&2
      return 1
      ;;
  esac
}

prompt_for_custom_run() {
  local provider_choice provider default_model model required_api_key

  echo "Choose provider:"
  echo "  1) anthropic"
  echo "  2) openai"
  echo "  3) openresponses"
  read -r -p "Provider [1]: " provider_choice
  provider_choice="${provider_choice:-1}"

  case "${provider_choice}" in
    1)
      provider="anthropic"
      ;;
    2)
      provider="openai"
      ;;
    3)
      provider="openresponses"
      ;;
    *)
      echo "Invalid provider selection: ${provider_choice}" >&2
      exit 1
      ;;
  esac

  default_model="$(default_model_for_provider "${provider}")"
  read -r -p "Model [${default_model}]: " model
  model="${model:-${default_model}}"
  required_api_key="$(required_api_key_for_provider "${provider}")"
  selected_run_matrix=("${provider}|${model}|${required_api_key}")
}

prompt_for_run_selection() {
  if [[ ! -t 0 || ! -t 1 || "${GBASH_EVAL_PROMPT:-1}" == "0" ]]; then
    selected_run_matrix=("${run_matrix[@]}")
    return
  fi

  local option_count=1
  local entry provider model required_api_key choice

  echo "Choose benchmark target:"
  echo "  1) all configured benchmarks"
  for entry in "${run_matrix[@]}"; do
    IFS='|' read -r provider model required_api_key <<<"${entry}"
    option_count=$((option_count + 1))
    echo "  ${option_count}) ${provider}/${model}"
  done
  option_count=$((option_count + 1))
  echo "  ${option_count}) custom single run"

  read -r -p "Selection [1]: " choice
  choice="${choice:-1}"

  case "${choice}" in
    1)
      selected_run_matrix=("${run_matrix[@]}")
      ;;
    $(( ${#run_matrix[@]} + 2 )))
      prompt_for_custom_run
      ;;
    *)
      if [[ "${choice}" =~ ^[0-9]+$ ]] && (( choice >= 2 && choice <= ${#run_matrix[@]} + 1 )); then
        selected_run_matrix=("${run_matrix[$((choice - 2))]}")
      else
        echo "Invalid selection: ${choice}" >&2
        exit 1
      fi
      ;;
  esac
}

print_selected_run_matrix() {
  echo "Selected benchmark runs:"
  printf '  %-14s %-28s %s\n' "provider" "model" "required_api_key"
  local entry provider model required_api_key
  for entry in "${selected_run_matrix[@]}"; do
    IFS='|' read -r provider model required_api_key <<<"${entry}"
    printf '  %-14s %-28s %s\n' "${provider}" "${model}" "${required_api_key}"
  done
}

require_exported_api_keys() {
  local entry provider model required_api_key
  for entry in "${selected_run_matrix[@]}"; do
    IFS='|' read -r provider model required_api_key <<<"${entry}"
    if [[ -z "${!required_api_key:-}" ]]; then
      echo "${required_api_key} must be exported before running ${provider}/${model}" >&2
      exit 1
    fi
  done
}

run_eval() {
  local provider="$1"
  local model="$2"
  local moniker
  moniker="$(moniker_for "${moniker_prefix}-${provider}-${model}")"

  echo "==> ${provider}/${model}"
  (
    cd "${repo_root}"
    go run ./examples/gbash-eval run \
      --dataset "${dataset}" \
      --provider "${provider}" \
      --model "${model}" \
      --save \
      --output "${output_dir}" \
      --moniker "${moniker}"
  )
}

print_run_matrix
prompt_for_run_selection
print_selected_run_matrix
require_exported_api_keys
echo

for entry in "${selected_run_matrix[@]}"; do
  IFS='|' read -r provider model required_api_key <<<"${entry}"
  run_eval "${provider}" "${model}"
  echo
done
