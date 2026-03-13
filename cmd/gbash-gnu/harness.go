package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ewhauser/gbash/internal/compatshims"
)

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

func prepareProgramDir(workDir, gbashBin string, programs []string) error {
	srcDir := filepath.Join(workDir, "src")
	for _, name := range programs {
		if err := os.RemoveAll(filepath.Join(srcDir, name)); err != nil {
			return err
		}
	}
	if err := compatshims.SymlinkCommands(srcDir, gbashBin, programs); err != nil {
		return err
	}
	helperShells := compatHelperShells()
	if err := compatshims.SymlinkCommands(srcDir, gbashBin, helperShells); err != nil {
		return err
	}
	if err := compatshims.SymlinkCommands(srcDir, gbashBin, []string{"ginstall"}); err != nil {
		return err
	}
	return writeGNUProgramList(workDir, programs)
}

func compatHelperShells() []string {
	return []string{"bash", "sh"}
}

func compatConfigShellPath(workDir string) (string, error) {
	path := filepath.Join(workDir, "src", "bash")
	if err := ensureExecutable(path); err != nil {
		return "", fmt.Errorf("prepare compat config shell: %w", err)
	}
	return path, nil
}

func writeGNUProgramList(workDir string, programs []string) error {
	hookDir := compatHarnessDir(workDir)
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return err
	}
	return writeLines(filepath.Join(hookDir, "gnu-programs.txt"), programs)
}

func installCompatTestHooks(workDir, gbashBin string) error {
	if err := writeCompatRelinkScript(workDir, gbashBin); err != nil {
		return fmt.Errorf("write relink hook: %w", err)
	}
	if err := patchCompatTestsEnvironment(workDir); err != nil {
		return fmt.Errorf("patch TESTS_ENVIRONMENT: %w", err)
	}
	if err := patchCompatInitSetup(workDir); err != nil {
		return fmt.Errorf("patch tests/init.sh setup: %w", err)
	}
	return nil
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

func writeLines(path string, lines []string) error {
	sorted := append([]string(nil), lines...)
	sort.Strings(sorted)
	data := strings.Join(sorted, "\n")
	if data != "" {
		data += "\n"
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

func shellSingleQuoteForScript(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func compatHarnessDir(workDir string) string {
	return filepath.Join(workDir, "build-aux", "gbash-harness")
}

func writeCompatRelinkScript(workDir, gbashBin string) error {
	hookDir := compatHarnessDir(workDir)
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return err
	}
	data, err := renderRelinkScript(gbashBin)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(hookDir, "relink.sh"), data, 0o755)
}

func patchCompatTestsEnvironment(workDir string) error {
	makefilePath := filepath.Join(workDir, "Makefile")
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		return err
	}
	contents := string(data)
	if strings.Contains(contents, "gbash-harness/relink.sh") {
		return nil
	}
	insert, err := loadAssetText("patches/tests_environment.txt")
	if err != nil {
		return err
	}
	updated := strings.Replace(contents, "TESTS_ENVIRONMENT = \\\n", insert, 1)
	if updated == contents {
		return fmt.Errorf("TESTS_ENVIRONMENT declaration not found in %s", makefilePath)
	}
	return os.WriteFile(makefilePath, []byte(updated), 0o644)
}

func patchCompatInitSetup(workDir string) error {
	initPath := filepath.Join(workDir, "tests", "init.sh")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return err
	}
	contents := string(data)
	if strings.Contains(contents, "jbgo_path_before_setup_=$PATH") {
		return nil
	}
	replacement, err := loadAssetText("patches/tests_init_setup.txt")
	if err != nil {
		return err
	}
	const target = `setup_ "$@"
# This trap is here, rather than in the setup_ function, because some
# shells run the exit trap at shell function exit, rather than script exit.
trap remove_tmp_ EXIT
`
	updated := strings.Replace(contents, target, replacement, 1)
	if updated == contents {
		return fmt.Errorf("tests/init.sh setup block not found in %s", initPath)
	}
	return os.WriteFile(initPath, []byte(updated), 0o644)
}
