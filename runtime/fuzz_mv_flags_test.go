package runtime

import "testing"

func FuzzMVFlagsCommand(f *testing.F) {
	rt := newFuzzRuntime(f)

	seeds := [][]byte{
		[]byte("move this\n"),
		[]byte("rename me\n"),
		[]byte("force accepted\n"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rawData []byte) {
		session := newFuzzSession(t, rt)
		srcPath := "/tmp/mv-src.txt"
		movePath := "/tmp/mv-move.txt"
		forcePath := "/tmp/mv-force.txt"
		destDir := "/tmp/mv-dst"
		occupiedPath := "/tmp/mv-occupied.txt"
		forcedPath := "/tmp/mv-forced.txt"

		writeSessionFile(t, session, srcPath, normalizeFuzzText(rawData))
		writeSessionFile(t, session, movePath, normalizeFuzzText(rawData))
		writeSessionFile(t, session, forcePath, normalizeFuzzText(rawData))
		writeSessionFile(t, session, occupiedPath, []byte("keep\n"))

		script := []byte(
			"mkdir -p " + shellQuote(destDir) + "\n" +
				"cp " + shellQuote(srcPath) + " " + shellQuote(destDir+"/mv-src.txt") + " || true\n" +
				"mv -n " + shellQuote(srcPath) + " " + shellQuote(destDir+"/mv-src.txt") + " >/tmp/mv-skip.txt || true\n" +
				"mv --verbose " + shellQuote(movePath) + " " + shellQuote(destDir) + " >/tmp/mv-verbose.txt || true\n" +
				"mv -f " + shellQuote(forcePath) + " " + shellQuote(forcedPath) + " >/tmp/mv-force.txt || true\n" +
				"cp " + shellQuote(forcedPath) + " " + shellQuote("/tmp/mv-skip-source.txt") + " || true\n" +
				"mv --force --no-clobber " + shellQuote("/tmp/mv-skip-source.txt") + " " + shellQuote(occupiedPath) + " >/tmp/mv-long.txt || true\n",
		)

		result, err := runFuzzSessionScript(t, session, script)
		assertSuccessfulFuzzExecution(t, script, result, err)
	})
}
