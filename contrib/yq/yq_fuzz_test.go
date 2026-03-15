package yq

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	gbruntime "github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/internal/fuzztest"
	"github.com/ewhauser/gbash/policy"
)

const (
	fuzzMaxDataBytes = 4 << 10
)

func newFuzzRuntime(tb testing.TB) *gbruntime.Runtime {
	tb.Helper()
	return fuzztest.NewRuntime(tb, fuzztest.RuntimeOptions{
		Registry: newYQRegistry(tb),
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:  []string{"/"},
			WriteRoots: []string{"/"},
			Limits: policy.Limits{
				MaxCommandCount:      200,
				MaxGlobOperations:    2000,
				MaxLoopIterations:    200,
				MaxSubstitutionDepth: 8,
				MaxStdoutBytes:       16 << 10,
				MaxStderrBytes:       16 << 10,
				MaxFileBytes:         128 << 10,
			},
		}),
	})
}

func newFuzzSession(tb testing.TB, rt *gbruntime.Runtime) *gbruntime.Session {
	tb.Helper()
	return fuzztest.NewSession(tb, rt)
}

func runFuzzSessionScript(t *testing.T, session *gbruntime.Session, script []byte) (*gbruntime.ExecutionResult, error) {
	t.Helper()
	return fuzztest.RunSessionScript(t, session, "yq-fuzz.sh", script)
}

func assertSuccessfulFuzzExecution(t *testing.T, script []byte, result *gbruntime.ExecutionResult, err error) {
	t.Helper()
	fuzztest.AssertSuccessfulFuzzExecution(t, script, result, err)
}

func clampFuzzData(data []byte) []byte {
	if len(data) <= fuzzMaxDataBytes {
		return data
	}
	return data[:fuzzMaxDataBytes]
}

func sanitizeFuzzToken(raw string) string {
	raw = strings.ToValidUTF8(raw, "")
	raw = strings.ReplaceAll(raw, "\x00", "")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "value"
	}
	if len(raw) > 32 {
		raw = raw[:32]
	}
	return raw
}

func addStructuredDataSeeds(f *testing.F) {
	f.Helper()

	seeds := []struct {
		value string
		raw   []byte
	}{
		{"alpha", []byte(`{"value":"alpha","items":[1,2,3]}`)},
		{"beta", []byte(`{"value":"beta","items":["x","y"]}`)},
		{"gamma", []byte(`not-json`)},
	}
	for _, seed := range seeds {
		f.Add(seed.value, seed.raw)
	}
}

func prepareStructuredDataFixtures(t *testing.T, session *gbruntime.Session, rawValue string, rawJSON []byte) {
	t.Helper()

	value := sanitizeFuzzToken(rawValue)
	validDoc := map[string]any{
		"value": value,
		"items": []string{value, strings.ToUpper(value)},
	}
	validBytes, err := json.Marshal(validDoc)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	writeSessionFile(t, session, "/tmp/input.json", validBytes)
	writeSessionFile(t, session, "/tmp/input.yaml", fmt.Appendf(nil, "value: %s\nitems:\n  - %s\n  - %s\n", value, value, strings.ToUpper(value)))
	writeSessionFile(t, session, "/tmp/raw.json", clampFuzzData(rawJSON))
}

func FuzzYQCommands(f *testing.F) {
	rt := newFuzzRuntime(f)
	addStructuredDataSeeds(f)

	f.Fuzz(func(t *testing.T, rawValue string, rawJSON []byte) {
		session := newFuzzSession(t, rt)
		prepareStructuredDataFixtures(t, session, rawValue, rawJSON)

		script := []byte(
			"yq -p yaml -o yaml '.value' /tmp/input.yaml >/tmp/yq-value.txt\n" +
				"yq -p json -o json '.items' /tmp/input.json >/tmp/yq-items.txt\n" +
				"yq -n '.value = \"built\"' >/tmp/yq-build.txt\n",
		)

		result, err := runFuzzSessionScript(t, session, script)
		assertSuccessfulFuzzExecution(t, script, result, err)
	})
}
