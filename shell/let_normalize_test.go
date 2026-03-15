package shell

import "testing"

func TestNormalizeLetCommandsRewritesCommandPositionLet(t *testing.T) {
	t.Parallel()

	script := "" +
		"let $a+=1\n" +
		"FOO=1 let ${a}+=1\n" +
		"if true; then let $a+=1; elif let ++$a; then :; else let $a++; fi\n" +
		"while false; do let $a+=1; done\n"

	want := "" +
		letHelperCommandAlias + " $a+=1\n" +
		"FOO=1 " + letHelperCommandAlias + " ${a}+=1\n" +
		"if true; then " + letHelperCommandAlias + " $a+=1; elif " + letHelperCommandAlias + " ++$a; then :; else " + letHelperCommandAlias + " $a++; fi\n" +
		"while false; do " + letHelperCommandAlias + " $a+=1; done\n"

	if got := normalizeLetCommands(script); got != want {
		t.Fatalf("normalizeLetCommands() = %q, want %q", got, want)
	}
}

func TestNormalizeLetCommandsSkipsNonCommandContexts(t *testing.T) {
	t.Parallel()

	script := "" +
		"command let $a+=1\n" +
		"builtin let $a+=1\n" +
		"echo let\n" +
		"let(){ :; }\n" +
		"let () { :; }\n" +
		"printf '%s\\n' 'let $a+=1'\n" +
		"cat <<EOF\n" +
		"let $a+=1\n" +
		"EOF\n" +
		"((let<3))\n" +
		"for ((let=0; let<3; let++)); do :; done\n" +
		"echo $(let $a+=1)\n" +
		"echo `let $a+=1`\n" +
		"echo \"let $a+=1\"\n"

	if got := normalizeLetCommands(script); got != script {
		t.Fatalf("normalizeLetCommands() = %q, want unchanged script", got)
	}
}
