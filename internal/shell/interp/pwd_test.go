package interp

import "testing"

func TestPWDVarsStayExported(t *testing.T) {
	t.Parallel()

	runner, err := NewRunner(&RunnerConfig{Dir: "/tmp"})
	if err != nil {
		t.Fatalf("NewRunner error = %v", err)
	}
	runner.Reset()

	if got := runner.lookupVar("PWD"); !got.Exported || got.String() != "/tmp" {
		t.Fatalf("PWD = %#v, want exported /tmp", got)
	}

	runner.setCurrentDir("/var", "/var", "/tmp")

	if got := runner.lookupVar("PWD"); !got.Exported || got.String() != "/var" {
		t.Fatalf("PWD after cd = %#v, want exported /var", got)
	}
	if got := runner.lookupVar("OLDPWD"); !got.Exported || got.String() != "/tmp" {
		t.Fatalf("OLDPWD after cd = %#v, want exported /tmp", got)
	}
}
