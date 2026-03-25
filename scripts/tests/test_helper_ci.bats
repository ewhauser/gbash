#!/usr/bin/env bats

load test_helper

setup() {
	HOST_TMP_ROOT="$(mktemp -d "${REPO_ROOT}/.bats-host-tmp.XXXXXX")"
	TEST_TEMP_DIR="$(TMPDIR="${HOST_TMP_ROOT}" mktemp -d)"
	SANDBOX="${TEST_TEMP_DIR}/sandbox"
	STUB_BIN="${SANDBOX}/bin"
	mkdir -p "${STUB_BIN}"
}

teardown() {
	rm -rf "${HOST_TMP_ROOT}" "${TEST_TEMP_DIR}"
}

@test "run_gbash accepts sandboxes under a custom host temp root" {
	run_gbash "printf '%s' ok"
	[ "$status" -eq 0 ]
	[ "$output" = "ok" ]
}

@test "run_gbash canonicalizes a relative host temp root" {
	relative_tmp_root=".bats-relative-host-tmp"
	mkdir -p "${relative_tmp_root}"
	TEST_TEMP_DIR="$(TMPDIR="${relative_tmp_root}" mktemp -d)"
	SANDBOX="${TEST_TEMP_DIR}/sandbox"
	STUB_BIN="${SANDBOX}/bin"
	mkdir -p "${STUB_BIN}"

	run_gbash "printf '%s' ok"
	[ "$status" -eq 0 ]
	[ "$output" = "ok" ]
	rm -rf "${relative_tmp_root}"
}
