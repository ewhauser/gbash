package main

import (
	"bytes"
	"embed"
	"encoding/json"
	templatepkg "text/template"
)

//go:embed manifest.json patches/*.txt templates/*.tmpl
var assetFS embed.FS

var relinkTemplate = templatepkg.Must(templatepkg.New("relink.sh.tmpl").ParseFS(assetFS, "templates/relink.sh.tmpl"))

var (
	testsEnvironmentPatch = mustReadEmbeddedText("patches/tests_environment.txt")
	testsInitSetupPatch   = mustReadEmbeddedText("patches/tests_init_setup.txt")
)

type relinkScriptView struct {
	GBashBinQuoted string
}

func loadManifest() (*manifest, error) {
	data, err := assetFS.ReadFile("manifest.json")
	if err != nil {
		return nil, err
	}
	var out manifest
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func renderRelinkScript(gbashBin string) ([]byte, error) {
	var buf bytes.Buffer
	view := relinkScriptView{GBashBinQuoted: shellSingleQuoteForScript(gbashBin)}
	if err := relinkTemplate.ExecuteTemplate(&buf, "relink.sh.tmpl", view); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mustReadEmbeddedText(path string) string {
	data, err := assetFS.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(data)
}
