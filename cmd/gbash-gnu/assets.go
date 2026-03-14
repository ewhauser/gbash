package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"text/template"
)

//go:embed manifest.json patches/*.txt templates/*.tmpl
var assetFS embed.FS

var (
	launcherScriptTemplate       = template.Must(template.New("launcher.sh.tmpl").ParseFS(assetFS, "templates/launcher.sh.tmpl"))
	programWrapperScriptTemplate = template.Must(template.New("wrapper.sh.tmpl").ParseFS(assetFS, "templates/wrapper.sh.tmpl"))
	relinkScriptTemplate         = template.Must(template.New("relink.sh.tmpl").ParseFS(assetFS, "templates/relink.sh.tmpl"))
)

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

func renderLauncherScript(gbashBin string) ([]byte, error) {
	var buf bytes.Buffer
	if err := launcherScriptTemplate.Execute(&buf, struct {
		GBashBinQuoted string
	}{
		GBashBinQuoted: shellSingleQuoteForScript(gbashBin),
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderProgramWrapperScript(commandName string) ([]byte, error) {
	var buf bytes.Buffer
	if err := programWrapperScriptTemplate.Execute(&buf, struct {
		CommandNameQuoted string
	}{
		CommandNameQuoted: shellSingleQuoteForScript(commandName),
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderShellWrapperScript() ([]byte, error) {
	var buf bytes.Buffer
	if err := programWrapperScriptTemplate.Execute(&buf, struct {
		CommandNameQuoted string
	}{}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderRelinkScript() ([]byte, error) {
	var buf bytes.Buffer
	if err := relinkScriptTemplate.Execute(&buf, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
