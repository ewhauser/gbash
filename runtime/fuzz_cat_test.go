package runtime

import (
	"fmt"
	"testing"
)

func FuzzCatCommand(f *testing.F) {
	rt := newFuzzRuntime(f)

	seeds := [][]byte{
		[]byte("alpha\nbeta\n"),
		[]byte("one line only"),
		{0x00, 0x01, 0x02, '\n', 0xff},
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rawData []byte) {
		session := newFuzzSession(t, rt)
		inputPath := "/tmp/cat-input.txt"

		writeSessionFile(t, session, inputPath, clampFuzzData(rawData))

		script := fmt.Appendf(nil,
			"cat --number %s >/tmp/cat-numbered.txt\ncat -n %s >/tmp/cat-short.txt\n",
			shellQuote(inputPath),
			shellQuote(inputPath),
		)

		result, err := runFuzzSessionScript(t, session, script)
		assertSuccessfulFuzzExecution(t, script, result, err)
	})
}
