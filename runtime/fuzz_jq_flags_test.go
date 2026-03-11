package runtime

import (
	"encoding/json"
	"testing"
)

func FuzzJQCompatibilityFlags(f *testing.F) {
	rt := newFuzzRuntime(f)

	seeds := [][]byte{
		[]byte("alpha"),
		[]byte("beta"),
		[]byte("unicode-\xce\xa9"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rawData []byte) {
		session := newFuzzSession(t, rt)
		inputPath := "/tmp/jq-compat.json"
		doc := map[string]any{
			"value": sanitizeFuzzToken(string(rawData)),
			"items": []string{sanitizeFuzzToken(string(rawData)), "beta"},
		}
		jsonBytes, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		writeSessionFile(t, session, inputPath, jsonBytes)

		script := []byte(
			"jq --compact --ascii --color --monochrome '.' " + shellQuote(inputPath) + " >/tmp/jq-long.txt || true\n" +
				"jq -aCMc '.' " + shellQuote(inputPath) + " >/tmp/jq-short.txt || true\n",
		)

		result, err := runFuzzSessionScript(t, session, script)
		assertSuccessfulFuzzExecution(t, script, result, err)
	})
}
