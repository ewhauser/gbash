package interp

import "testing"

func TestShoptQueryAndPrintFlags(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
shopt -q extglob
printf 'q0=%d\n' "$?"
shopt -p extglob
printf 'p0=%d\n' "$?"
shopt extglob
printf 'd0=%d\n' "$?"
shopt -s extglob
shopt -q extglob
printf 'q1=%d\n' "$?"
shopt -p extglob
printf 'p1=%d\n' "$?"
shopt extglob
printf 'd1=%d\n' "$?"
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const want = "" +
		"q0=1\n" +
		"shopt -u extglob\n" +
		"p0=1\n" +
		"extglob\toff\n" +
		"d0=1\n" +
		"q1=0\n" +
		"shopt -s extglob\n" +
		"p1=0\n" +
		"extglob\ton\n" +
		"d1=0\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestShoptStubOptionsCanToggle(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
shopt -u progcomp hostcomplete
printf 'unset=%d\n' "$?"
shopt -q progcomp hostcomplete
printf 'query0=%d\n' "$?"
shopt -p progcomp hostcomplete
printf 'print0=%d\n' "$?"
shopt -s progcomp hostcomplete
printf 'set=%d\n' "$?"
shopt -q progcomp hostcomplete
printf 'query1=%d\n' "$?"
shopt -p progcomp hostcomplete
printf 'print1=%d\n' "$?"
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const want = "" +
		"unset=0\n" +
		"query0=1\n" +
		"shopt -u progcomp\n" +
		"shopt -u hostcomplete\n" +
		"print0=1\n" +
		"set=0\n" +
		"query1=0\n" +
		"shopt -s progcomp\n" +
		"shopt -s hostcomplete\n" +
		"print1=0\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestShoptInvalidOptionNameMatchesBash(t *testing.T) {
	t.Parallel()

	_, stderr, err := runInterpScript(t, `
shopt -s strict_array
printf 'status=%d\n' "$?"
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const want = "shopt: strict_array: invalid shell option name\n"
	if stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
}
