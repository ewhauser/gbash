package syntax

import (
	"strings"
	"testing"
	"time"
)

// A backquote in a case pattern stalled the lexer while the parser skipped
// tokens up to the end of the raw pattern, so even "case i in `;" spun
// forever (found by FuzzParsePrint).
func TestCasePatternBackquoteDoesNotHang(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src     string
		wantErr bool
	}{
		{src: "case i in `;", wantErr: true},
		{src: "case i in `", wantErr: true},
		{src: `case i in "` + "`" + `";`, wantErr: true},
		{src: "case i in `echo i`) echo y;; esac"},
		{src: "case ab in a`echo b`) echo y;; esac"},
		{src: "case i in `echo j`|`echo i`) echo y;; esac"},
		{src: "case i in x) ;; `echo i`) echo second;; esac"},
		{src: "case i in `echo i` ) echo y;; esac"},
	}
	for _, tc := range tests {
		done := make(chan error, 1)
		go func() {
			_, err := NewParser(KeepComments(true)).Parse(strings.NewReader(tc.src), "")
			done <- err
		}()
		select {
		case err := <-done:
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("Parse(%q) error = %v, want error: %v", tc.src, err, tc.wantErr)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("Parse(%q) did not finish", tc.src)
		}
	}
}

func TestCasePatternBackquoteParsesCmdSubst(t *testing.T) {
	t.Parallel()
	f, err := NewParser().Parse(strings.NewReader("case i in `echo i`) echo y;; esac"), "")
	if err != nil {
		t.Fatal(err)
	}
	cc := f.Stmts[0].Cmd.(*CaseClause)
	var found bool
	Walk(cc.Items[0].Patterns[0], func(n Node) bool {
		if cs, ok := n.(*CmdSubst); ok && cs.Backquotes {
			found = true
		}
		return true
	})
	if !found {
		t.Fatalf("case pattern has no backquoted command substitution")
	}
}
