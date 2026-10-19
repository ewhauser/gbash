package builtins_test

import (
	"testing"
	"time"
)

func TestMktempAllXTemplateStaysInsideTmpdir(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := execWithTimeout(t, session, ""+
		"a=$(mktemp -p /tmp XXXXXX); echo \"${a%/*}\"\n"+
		"b=$(mktemp --tmpdir=/tmp XXXXXX); echo \"${b%/*}\"\n"+
		"c=$(TMPDIR=/tmp mktemp -t XXXXXX); echo \"${c%/*}\"\n"+
		"d=$(mktemp -d -p /tmp/ XXXXXX); echo \"${d%/*}\"; test -d \"$d\" && echo dir\n"+
		"ls / | grep -c '^tmp.' || true\n", 10*time.Second)
	if got, want := result.Stdout, "/tmp\n/tmp\n/tmp\n/tmp\ndir\n0\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}
