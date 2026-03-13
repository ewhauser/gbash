package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"text/template"
)

//go:embed manifest.json patches/*.txt templates/*.tmpl
var assetFS embed.FS

var relinkScriptTemplate = template.Must(template.New("relink.sh.tmpl").ParseFS(assetFS, "templates/relink.sh.tmpl"))

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

func loadAssetText(path string) (string, error) {
	data, err := assetFS.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func renderRelinkScript(gbashBin string) ([]byte, error) {
	var buf bytes.Buffer
	if err := relinkScriptTemplate.Execute(&buf, struct {
		GBashBinQuoted string
	}{
		GBashBinQuoted: shellSingleQuoteForScript(gbashBin),
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
