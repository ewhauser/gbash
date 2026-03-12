package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	gbcommands "github.com/ewhauser/gbash/commands"
	"github.com/ewhauser/gbash/internal/compatshims"
)

const legacyTestsEnvironmentHook = "  $(SHELL) '$(abs_top_builddir)/build-aux/jbgo-harness/relink.sh' '$(abs_top_builddir)/src' || exit $$?; \\\n"

func listGNUPrograms(ctx context.Context, workDir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, filepath.Join(workDir, "build-aux", "gen-lists-of-programs.sh"), "--list-progs")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list GNU programs: %w", err)
	}
	lines := strings.Fields(string(out))
	sort.Strings(lines)
	return lines, nil
}

func prepareProgramDir(workDir, gbashBin string, programs []string, supported map[string]struct{}) error {
	srcDir := filepath.Join(workDir, "src")
	supportedNames := make([]string, 0)
	unsupportedNames := make([]string, 0)
	for _, name := range programs {
		path := filepath.Join(srcDir, name)
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if _, ok := supported[name]; ok {
			supportedNames = append(supportedNames, name)
		} else {
			unsupportedNames = append(unsupportedNames, name)
		}
	}
	if err := compatshims.SymlinkCommands(srcDir, gbashBin, supportedNames); err != nil {
		return err
	}
	if err := compatshims.WriteUnsupportedStubs(srcDir, unsupportedNames); err != nil {
		return err
	}
	helperShells := compatHelperShells(supported)
	if err := compatshims.SymlinkCommands(srcDir, gbashBin, helperShells); err != nil {
		return err
	}
	supportedNames = appendUniqueStrings(supportedNames, helperShells...)
	if _, ok := supported["install"]; ok {
		supportedNames = append(supportedNames, "ginstall")
		if err := compatshims.SymlinkCommands(srcDir, gbashBin, []string{"ginstall"}); err != nil {
			return err
		}
	} else {
		unsupportedNames = append(unsupportedNames, "ginstall")
		if err := compatshims.WriteUnsupportedStubs(srcDir, []string{"ginstall"}); err != nil {
			return err
		}
	}
	return installTestRelinkHook(workDir, gbashBin, supportedNames, unsupportedNames)
}

func compatHelperShells(supported map[string]struct{}) []string {
	names := make([]string, 0, 2)
	for _, name := range []string{"bash", "sh"} {
		if _, ok := supported[name]; ok {
			names = append(names, name)
		}
	}
	return names
}

func appendUniqueStrings(items []string, values ...string) []string {
	seen := make(map[string]struct{}, len(items)+len(values))
	for _, item := range items {
		seen[item] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}
	return items
}

func compatConfigShellPath(workDir string) (string, error) {
	path := filepath.Join(workDir, "src", "bash")
	if err := ensureExecutable(path); err != nil {
		return "", fmt.Errorf("prepare compat config shell: %w", err)
	}
	return path, nil
}

func implementedGNUProgramSet() map[string]struct{} {
	supported := make(map[string]struct{})
	for _, name := range gbcommands.DefaultRegistry().Names() {
		supported[name] = struct{}{}
	}
	return supported
}

func disableCheckRebuild(workDir string) error {
	makefilePath := filepath.Join(workDir, "Makefile")
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		return err
	}
	updated := strings.Replace(string(data), "check-am: all-am", "check-am:", 1)
	if updated == string(data) {
		return nil
	}
	return os.WriteFile(makefilePath, []byte(updated), 0o644)
}

func installTestRelinkHook(workDir, gbashBin string, supportedNames, unsupportedNames []string) error {
	hookDir := filepath.Join(workDir, "build-aux", "gbash-harness")
	_ = os.RemoveAll(filepath.Join(workDir, "build-aux", "jbgo-harness"))
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return err
	}
	if err := writeLines(filepath.Join(hookDir, "supported-programs.txt"), supportedNames); err != nil {
		return err
	}
	if err := writeLines(filepath.Join(hookDir, "unsupported-programs.txt"), unsupportedNames); err != nil {
		return err
	}

	scriptPath := filepath.Join(hookDir, "relink.sh")
	script, err := renderRelinkScript(gbashBin)
	if err != nil {
		return err
	}
	if err := os.WriteFile(scriptPath, script, 0o755); err != nil {
		return err
	}
	if err := patchTestsEnvironment(filepath.Join(workDir, "Makefile")); err != nil {
		return err
	}
	return patchTestInitSetupPath(filepath.Join(workDir, "tests", "init.sh"))
}

func writeLines(path string, lines []string) error {
	sorted := append([]string(nil), lines...)
	sort.Strings(sorted)
	data := strings.Join(sorted, "\n")
	if data != "" {
		data += "\n"
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

func patchTestsEnvironment(makefilePath string) error {
	return applyFilePatch(&filePatch{
		path:                makefilePath,
		needle:              "TESTS_ENVIRONMENT = \\\n",
		replacement:         testsEnvironmentPatch,
		idempotencyMarker:   "build-aux/gbash-harness/relink.sh",
		removeBeforeReplace: []string{legacyTestsEnvironmentHook},
		missingMarkerError:  "patch TESTS_ENVIRONMENT: marker not found",
	})
}

func patchTestInitSetupPath(initPath string) error {
	return applyFilePatch(&filePatch{
		path:               initPath,
		needle:             "setup_ \"$@\"\n# This trap is here, rather than in the setup_ function, because some\n# shells run the exit trap at shell function exit, rather than script exit.\ntrap remove_tmp_ EXIT\n",
		replacement:        testsInitSetupPatch,
		idempotencyMarker:  "jbgo_path_before_setup_=$PATH",
		missingMarkerError: "patch tests/init.sh setup PATH: marker not found",
	})
}

type filePatch struct {
	path                string
	needle              string
	replacement         string
	idempotencyMarker   string
	removeBeforeReplace []string
	missingMarkerError  string
}

func applyFilePatch(patch *filePatch) error {
	data, err := os.ReadFile(patch.path)
	if err != nil {
		return err
	}
	contents := string(data)
	updated := contents
	for _, removal := range patch.removeBeforeReplace {
		updated = strings.ReplaceAll(updated, removal, "")
	}
	if patch.idempotencyMarker != "" && strings.Contains(updated, patch.idempotencyMarker) {
		if updated == contents {
			return nil
		}
		return os.WriteFile(patch.path, []byte(updated), 0o644)
	}
	next := strings.Replace(updated, patch.needle, patch.replacement, 1)
	if next == updated {
		return fmt.Errorf("%s", patch.missingMarkerError)
	}
	if next == contents {
		return nil
	}
	return os.WriteFile(patch.path, []byte(next), 0o644)
}

func shellSingleQuoteForScript(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
