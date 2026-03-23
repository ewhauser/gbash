package gbasheval

import (
	"strings"
	"testing"
)

func TestEmbeddedSystemPrompts(t *testing.T) {
	t.Parallel()

	baseline := baselineSystemPrompt([]MockToolDef{{
		Name:        "check_inventory",
		Description: "Check inventory",
	}})
	if !strings.Contains(baseline, "You have access to the following tools:") {
		t.Fatalf("baselineSystemPrompt() = %q, want upstream baseline intro", baseline)
	}
	if !strings.Contains(baseline, "- check_inventory: Check inventory") {
		t.Fatalf("baselineSystemPrompt() = %q, want embedded tool listing", baseline)
	}

	scripted := scriptedSystemPrompt(ScriptingEvalTask{
		ID:            "scripted-discovery",
		DiscoveryMode: true,
	})
	if !strings.Contains(scripted, "discover --categories") {
		t.Fatalf("scriptedSystemPrompt() = %q, want upstream discover guidance", scripted)
	}
	if !strings.Contains(scripted, "help <tool> --json") {
		t.Fatalf("scriptedSystemPrompt() = %q, want upstream help guidance", scripted)
	}
	if !strings.Contains(scripted, "jq, sqlite3, yq") {
		t.Fatalf("scriptedSystemPrompt() = %q, want extras guidance", scripted)
	}
}
