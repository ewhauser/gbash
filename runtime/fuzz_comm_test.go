package runtime

import (
	"fmt"
	"testing"
)

func FuzzCommCommand(f *testing.F) {
	rt := newFuzzRuntime(f)

	f.Add([]byte("apple\nbanana\n"), []byte("banana\ncarrot\n"))
	f.Add([]byte("a\nb\nc\n"), []byte("b\nc\nd\n"))
	f.Add([]byte("same\n"), []byte("same\n"))

	f.Fuzz(func(t *testing.T, rawLeft, rawRight []byte) {
		session := newFuzzSession(t, rt)
		leftPath := "/tmp/comm-left.txt"
		rightPath := "/tmp/comm-right.txt"

		writeSessionFile(t, session, leftPath, normalizeFuzzText(rawLeft))
		writeSessionFile(t, session, rightPath, normalizeFuzzText(rawRight))

		script := fmt.Appendf(nil,
			"comm -1 %s %s >/tmp/comm-1.txt || true\ncomm -2 %s %s >/tmp/comm-2.txt || true\ncomm -3 %s %s >/tmp/comm-3.txt || true\n",
			shellQuote(leftPath),
			shellQuote(rightPath),
			shellQuote(leftPath),
			shellQuote(rightPath),
			shellQuote(leftPath),
			shellQuote(rightPath),
		)

		result, err := runFuzzSessionScript(t, session, script)
		assertSuccessfulFuzzExecution(t, script, result, err)
	})
}
