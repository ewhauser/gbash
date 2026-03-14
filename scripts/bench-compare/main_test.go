package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ewhauser/gbash"
)

func TestCreateWorkspaceFixture(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	summary, err := createWorkspaceFixture(root)
	if err != nil {
		t.Fatalf("createWorkspaceFixture() error = %v", err)
	}
	if summary.FileCount != 300 {
		t.Fatalf("FileCount = %d, want 300", summary.FileCount)
	}

	var countedFiles int
	var countedBytes int64
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		countedFiles++
		countedBytes += info.Size()
		return nil
	})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if countedFiles != summary.FileCount {
		t.Fatalf("walked files = %d, want %d", countedFiles, summary.FileCount)
	}
	if countedBytes != summary.TotalBytes {
		t.Fatalf("walked bytes = %d, want %d", countedBytes, summary.TotalBytes)
	}
}

func TestSummarizeDurations(t *testing.T) {
	stats, ok := summarizeDurations([]time.Duration{
		7 * time.Millisecond,
		1 * time.Millisecond,
		5 * time.Millisecond,
		9 * time.Millisecond,
		3 * time.Millisecond,
	})
	if !ok {
		t.Fatalf("summarizeDurations() ok = false, want true")
	}
	if got, want := time.Duration(stats.MinNanos), 1*time.Millisecond; got != want {
		t.Fatalf("Min = %s, want %s", got, want)
	}
	if got, want := time.Duration(stats.MedianNanos), 5*time.Millisecond; got != want {
		t.Fatalf("Median = %s, want %s", got, want)
	}
	if got, want := time.Duration(stats.P95Nanos), 9*time.Millisecond; got != want {
		t.Fatalf("P95 = %s, want %s", got, want)
	}
}

func TestBenchmarkScenarios(t *testing.T) {
	fixture := fixtureSummary{
		Root:       filepath.Join(string(filepath.Separator), "tmp", "workspace"),
		FileCount:  300,
		TotalBytes: 17100,
	}

	scenarios := benchmarkScenarios(fixture)
	if got, want := len(scenarios), 2; got != want {
		t.Fatalf("len(benchmarkScenarios()) = %d, want %d", got, want)
	}

	gotNames := make([]string, 0, len(scenarios))
	for _, scenario := range scenarios {
		gotNames = append(gotNames, scenario.Name)
	}
	wantNames := []string{
		"startup_echo",
		"workspace_inventory",
	}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("scenario names = %q, want %q", gotNames, wantNames)
	}

	for _, index := range []int{0, 1} {
		if got, want := scenarios[index].WarmupScript, "true\n"; got != want {
			t.Fatalf("scenarios[%d].WarmupScript = %q, want %q", index, got, want)
		}
	}
	if !scenarios[1].Workspace {
		t.Fatalf("workspace scenario should mount the generated fixture")
	}
	if got, want := scenarios[1].ExpectedStdout, "300\n"; got != want {
		t.Fatalf("workspace ExpectedStdout = %q, want %q", got, want)
	}
	if scenarios[1].Fixture == nil {
		t.Fatalf("workspace Fixture = nil, want fixture summary")
	}
	if got, want := scenarios[1].Fixture.Root, fixture.Root; got != want {
		t.Fatalf("workspace Fixture.Root = %q, want %q", got, want)
	}
}

func TestRenderTextReportAndJSON(t *testing.T) {
	report := benchmarkReport{
		GeneratedAt:  "2026-03-13T00:00:00Z",
		Runs:         2,
		JustBashSpec: "just-bash@2.13.0",
		Scenarios: []scenarioReport{
			{
				Name:           "startup_echo",
				Description:    "Process start plus one simple command.",
				Command:        "echo benchmark",
				ExpectedStdout: "benchmark\n",
				Results: []runtimeReport{
					{
						Name:              "gbash",
						ArtifactSizeBytes: 5 << 20,
						SuccessCount:      2,
						Stats: &latencyStats{
							MinNanos:    int64(time.Millisecond),
							MedianNanos: int64(2 * time.Millisecond),
							P95Nanos:    int64(3 * time.Millisecond),
						},
						Trials: []trialResult{
							{Index: 1, DurationNanos: int64(time.Millisecond), ExitCode: 0, Success: true, Stdout: "benchmark\n"},
							{Index: 2, DurationNanos: int64(3 * time.Millisecond), ExitCode: 0, Success: true, Stdout: "benchmark\n"},
						},
					},
				},
			},
		},
	}

	rendered := renderTextReport(report)
	if !strings.Contains(rendered, "Independent shell benchmark") {
		t.Fatalf("renderTextReport() missing report title:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[startup_echo]") {
		t.Fatalf("renderTextReport() missing scenario header:\n%s", rendered)
	}
	if !strings.Contains(rendered, "gbash: 2/2 successful") {
		t.Fatalf("renderTextReport() missing runtime summary:\n%s", rendered)
	}
	if !strings.Contains(rendered, "size=5.0 MiB") {
		t.Fatalf("renderTextReport() missing artifact size:\n%s", rendered)
	}
	if !strings.Contains(rendered, "median=2ms") {
		t.Fatalf("renderTextReport() missing latency stats:\n%s", rendered)
	}

	jsonPath := filepath.Join(t.TempDir(), "report.json")
	if err := writeJSONReport(jsonPath, report); err != nil {
		t.Fatalf("writeJSONReport() error = %v", err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "\"startup_echo\"") {
		t.Fatalf("JSON output missing scenario: %s", string(data))
	}
	if !strings.Contains(string(data), "\"artifact_size_bytes\": 5242880") {
		t.Fatalf("JSON output missing artifact size: %s", string(data))
	}
	if !strings.Contains(string(data), "\"just_bash_spec\": \"just-bash@2.13.0\"") {
		t.Fatalf("JSON output missing just-bash spec: %s", string(data))
	}
}

func TestGbashNodeWasmRuntime(t *testing.T) {
	repoRoot := filepath.Join(string(filepath.Separator), "repo")
	assetDir := filepath.Join(string(filepath.Separator), "tmp", "gbash-wasm")
	runtime := gbashNodeWasmRuntime(repoRoot, assetDir, 1234)

	cmd := runtime.Command(context.Background(), &scenarioConfig{
		Command: "echo benchmark\n",
	})
	if got, want := cmd.Dir, repoRoot; got != want {
		t.Fatalf("cmd.Dir = %q, want %q", got, want)
	}
	if got, want := filepath.Base(cmd.Path), "node"; got != want {
		t.Fatalf("cmd.Path = %q, want %q", got, want)
	}
	if got, want := runtime.ArtifactSizeBytes, int64(1234); got != want {
		t.Fatalf("runtime.ArtifactSizeBytes = %d, want %d", got, want)
	}
	wantArgs := []string{
		"node",
		filepath.Join(repoRoot, "scripts", "bench-compare", "gbash-wasm-runner.mjs"),
		"--wasm", filepath.Join(assetDir, "gbash.wasm"),
		"--wasm-exec", filepath.Join(assetDir, "wasm_exec.js"),
		"-c", "echo benchmark\n",
	}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Fatalf("cmd.Args = %q, want %q", cmd.Args, wantArgs)
	}
}

func TestGNUBashRuntime(t *testing.T) {
	repoRoot := filepath.Join(string(filepath.Separator), "repo")
	bashPath := filepath.Join(string(filepath.Separator), "bin", "bash")
	runtime := gnuBashRuntime(repoRoot, bashPath, 4321)

	cmd := runtime.Command(context.Background(), &scenarioConfig{
		Command: "echo benchmark\n",
	})
	if got, want := cmd.Path, bashPath; got != want {
		t.Fatalf("cmd.Path = %q, want %q", got, want)
	}
	if got, want := cmd.Dir, repoRoot; got != want {
		t.Fatalf("cmd.Dir = %q, want %q", got, want)
	}
	if got, want := runtime.ArtifactSizeBytes, int64(4321); got != want {
		t.Fatalf("runtime.ArtifactSizeBytes = %d, want %d", got, want)
	}
	wantArgs := []string{
		bashPath,
		"--noprofile",
		"--norc",
		"-c",
		"echo benchmark\n",
	}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Fatalf("cmd.Args = %q, want %q", cmd.Args, wantArgs)
	}
}

func TestGNUBashRuntimeWorkspace(t *testing.T) {
	repoRoot := filepath.Join(string(filepath.Separator), "repo")
	bashPath := filepath.Join(string(filepath.Separator), "bin", "bash")
	runtime := gnuBashRuntime(repoRoot, bashPath, 4321)

	fixtureRoot := filepath.Join(string(filepath.Separator), "tmp", "fixture")
	cmd := runtime.Command(context.Background(), &scenarioConfig{
		Command:   "find . -type f\n",
		Workspace: true,
		Fixture: &fixtureSummary{
			Root: fixtureRoot,
		},
	})
	if got, want := cmd.Dir, fixtureRoot; got != want {
		t.Fatalf("cmd.Dir = %q, want %q", got, want)
	}
}

func TestGbashExtrasRuntime(t *testing.T) {
	helperPath := filepath.Join(string(filepath.Separator), "tmp", "gbash-extras")
	runtime := gbashExtrasRuntime(helperPath, 5678)

	cmd := runtime.Command(context.Background(), &scenarioConfig{
		Command: "echo benchmark\n",
	})
	if got, want := cmd.Path, helperPath; got != want {
		t.Fatalf("cmd.Path = %q, want %q", got, want)
	}
	if got, want := runtime.ArtifactSizeBytes, int64(5678); got != want {
		t.Fatalf("runtime.ArtifactSizeBytes = %d, want %d", got, want)
	}
	wantArgs := []string{
		helperPath,
		"-c", "echo benchmark\n",
	}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Fatalf("cmd.Args = %q, want %q", cmd.Args, wantArgs)
	}
}

func TestGbashExtrasRuntimeWorkspace(t *testing.T) {
	helperPath := filepath.Join(string(filepath.Separator), "tmp", "gbash-extras")
	runtime := gbashExtrasRuntime(helperPath, 5678)

	cmd := runtime.Command(context.Background(), &scenarioConfig{
		Command:   "find . -type f\n",
		Workspace: true,
		Fixture: &fixtureSummary{
			Root: filepath.Join(string(filepath.Separator), "tmp", "fixture"),
		},
	})
	wantArgs := []string{
		helperPath,
		"--root", filepath.Join(string(filepath.Separator), "tmp", "fixture"),
		"--cwd", gbash.DefaultWorkspaceMountPoint,
		"-c", "find . -type f\n",
	}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Fatalf("cmd.Args = %q, want %q", cmd.Args, wantArgs)
	}
}

func TestGbashNodeWasmRuntimeWorkspace(t *testing.T) {
	repoRoot := filepath.Join(string(filepath.Separator), "repo")
	assetDir := filepath.Join(string(filepath.Separator), "tmp", "gbash-wasm")
	runtime := gbashNodeWasmRuntime(repoRoot, assetDir, 1234)

	cmd := runtime.Command(context.Background(), &scenarioConfig{
		Command:   "find . -type f\n",
		Workspace: true,
		Fixture: &fixtureSummary{
			Root: filepath.Join(string(filepath.Separator), "tmp", "fixture"),
		},
	})
	wantArgs := []string{
		"node",
		filepath.Join(repoRoot, "scripts", "bench-compare", "gbash-wasm-runner.mjs"),
		"--wasm", filepath.Join(assetDir, "gbash.wasm"),
		"--wasm-exec", filepath.Join(assetDir, "wasm_exec.js"),
		"--workspace", filepath.Join(string(filepath.Separator), "tmp", "fixture"),
		"--cwd", gbash.DefaultWorkspaceMountPoint,
		"-c", "find . -type f\n",
	}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Fatalf("cmd.Args = %q, want %q", cmd.Args, wantArgs)
	}
}

func TestFormatArtifactSize(t *testing.T) {
	tests := []struct {
		value int64
		want  string
	}{
		{0, "n/a"},
		{999, "999 B"},
		{1024, "1.0 KiB"},
		{5 << 20, "5.0 MiB"},
	}
	for _, tt := range tests {
		if got := formatArtifactSize(tt.value); got != tt.want {
			t.Fatalf("formatArtifactSize(%d) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestDirectorySize(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile(a.txt) error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatalf("Mkdir(nested) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "b.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile(b.txt) error = %v", err)
	}
	got, err := directorySize(root)
	if err != nil {
		t.Fatalf("directorySize() error = %v", err)
	}
	if want := int64(len("abc") + len("hello")); got != want {
		t.Fatalf("directorySize() = %d, want %d", got, want)
	}
}
