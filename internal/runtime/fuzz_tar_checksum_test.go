package runtime

import (
	"fmt"
	"path"
	"testing"
)

func FuzzTarCommand(f *testing.F) {
	rt := newFuzzRuntime(f)

	seeds := []struct {
		name string
		data []byte
	}{
		{"alpha", []byte("alpha\nbeta\n")},
		{"json", []byte("{\"value\":1}\n")},
		{"binary", []byte{0x00, 0x01, 0x02, 0xff, '\n'}},
	}
	for _, seed := range seeds {
		f.Add(seed.name, seed.data)
	}

	f.Fuzz(func(t *testing.T, rawName string, rawData []byte) {
		session := newFuzzSession(t, rt)
		name := sanitizeFuzzPathComponent(rawName)
		if name == "" {
			name = "item"
		}
		srcDir := "/tmp/tar-src"
		inputPath := path.Join(srcDir, name+".txt")
		restoredPath := path.Join("/tmp/tar-out", inputPath[1:])
		payload := clampFuzzData(rawData)

		writeSessionFile(t, session, inputPath, payload)

		script := fmt.Appendf(nil,
			"tar -cf /tmp/archive.tar %s\n"+
				"tar -tf /tmp/archive.tar >/tmp/tar.list\n"+
				"mkdir -p /tmp/tar-out\n"+
				"tar -xf /tmp/archive.tar -C /tmp/tar-out\n"+
				"cat %s >/tmp/tar-original.txt\n"+
				"cat %s >/tmp/tar-restored.txt\n",
			shellQuote(srcDir),
			shellQuote(inputPath),
			shellQuote(restoredPath),
		)

		result, err := runFuzzSessionScript(t, session, script)
		assertSuccessfulFuzzExecution(t, script, result, err)
	})
}

func FuzzChecksumCommands(f *testing.F) {
	rt := newFuzzRuntime(f)

	seeds := [][]byte{
		[]byte("alpha\n"),
		[]byte("hello,world\n"),
		{0x00, 0x01, 0x02, 0xff, '\n'},
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rawData []byte) {
		session := newFuzzSession(t, rt)
		inputPath := "/tmp/checksum.txt"
		payload := clampFuzzData(rawData)
		writeSessionFile(t, session, inputPath, payload)

		script := fmt.Appendf(nil,
			"md5sum %s >/tmp/md5.txt\n"+
				"sha256sum %s >/tmp/sha256.txt\n"+
				"cksum %s >/tmp/cksum.txt\n"+
				"md5sum %s >/tmp/md5-checksums.txt\n"+
				"md5sum -c /tmp/md5-checksums.txt >/tmp/md5-verify.txt\n"+
				"cksum -a sha2 -l 256 %s >/tmp/cksum-sha256.txt\n",
			shellQuote(inputPath),
			shellQuote(inputPath),
			shellQuote(inputPath),
			shellQuote(inputPath),
			shellQuote(inputPath),
		)

		result, err := runFuzzSessionScript(t, session, script)
		assertSuccessfulFuzzExecution(t, script, result, err)
	})
}
