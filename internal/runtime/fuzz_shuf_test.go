package runtime

import "testing"

func FuzzShufCommand(f *testing.F) {
	rt := newFuzzRuntime(f)

	seeds := []struct {
		data   []byte
		random []byte
	}{
		{[]byte("1\n2\n3\n"), []byte("seed-bytes-123456")},
		{[]byte("alpha\x00beta\x00"), []byte("short-seed")},
		{[]byte{}, []byte{}},
	}
	for _, seed := range seeds {
		f.Add(seed.data, seed.random)
	}

	f.Fuzz(func(t *testing.T, rawData []byte, rawRandom []byte) {
		session := newFuzzSession(t, rt)
		inputPath := "/tmp/shuf-input.txt"
		randomPath := "/tmp/shuf-random.bin"
		outputPath := "/tmp/shuf-output.txt"

		writeSessionFile(t, session, inputPath, clampFuzzData(rawData))
		writeSessionFile(t, session, randomPath, clampFuzzData(rawRandom))

		script := []byte(
			"shuf " + shellQuote(inputPath) + " >/tmp/shuf-file.txt || true\n" +
				"cat " + shellQuote(inputPath) + " | shuf >/tmp/shuf-stdin.txt || true\n" +
				"shuf -e alpha beta gamma >/tmp/shuf-echo.txt || true\n" +
				"shuf -i1-20 -n5 >/tmp/shuf-range.txt || true\n" +
				"shuf -r -n5 " + shellQuote(inputPath) + " >/tmp/shuf-repeat.txt || true\n" +
				"shuf -z " + shellQuote(inputPath) + " >/tmp/shuf-zero.txt || true\n" +
				"shuf --random-source=" + shellQuote(randomPath) + " " + shellQuote(inputPath) + " >/tmp/shuf-random.txt || true\n" +
				"shuf -o " + shellQuote(outputPath) + " " + shellQuote(inputPath) + " || true\n",
		)

		result, err := runFuzzSessionScript(t, session, script)
		assertSecureFuzzOutcome(t, script, result, err)
	})
}
