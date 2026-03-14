package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
	if err := writeProgramWrappers(srcDir, programs); err != nil {
		return err
	}
	if err := writeGNUProgramList(workDir, programs); err != nil {
		return err
	}
	return writeHarnessLauncher(workDir, gbashBin)
}

func harnessHelperShells() []string {
	return []string{"bash", "sh"}
}

func harnessConfigShellPath(workDir string) (string, error) {
	path := filepath.Join(workDir, "src", "bash")
	if err := ensureExecutable(path); err != nil {
		return "", fmt.Errorf("prepare harness config shell: %w", err)
	}
	return path, nil
}

func writeGNUProgramList(workDir string, programs []string) error {
	hookDir := harnessDir(workDir)
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return err
	}
	return writeLines(filepath.Join(hookDir, "gnu-programs.txt"), programs)
}

func installHarnessTestHooks(workDir, gbashBin string) error {
	if err := writeHarnessLauncher(workDir, gbashBin); err != nil {
		return fmt.Errorf("write launcher hook: %w", err)
	}
	if err := writeHarnessRelinkScript(workDir); err != nil {
		return fmt.Errorf("write relink hook: %w", err)
	}
	if err := patchHarnessTestsEnvironment(workDir); err != nil {
		return fmt.Errorf("patch TESTS_ENVIRONMENT: %w", err)
	}
	if err := patchHarnessInitSetup(workDir); err != nil {
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

func harnessDir(workDir string) string {
	return filepath.Join(workDir, "build-aux", "gbash-harness")
}

type harnessWrapperSpec struct {
	path        string
	commandName string
}

func writeProgramWrappers(srcDir string, programs []string) error {
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return err
	}
	for _, spec := range harnessWrapperSpecs(srcDir, programs) {
		var (
			data []byte
			err  error
		)
		if spec.commandName == "" {
			data, err = renderShellWrapperScript()
		} else {
			data, err = renderProgramWrapperScript(spec.commandName)
		}
		if err != nil {
			return err
		}
		if err := os.RemoveAll(spec.path); err != nil {
			return err
		}
		if err := os.WriteFile(spec.path, data, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func harnessWrapperSpecs(srcDir string, programs []string) []harnessWrapperSpec {
	specs := make([]harnessWrapperSpec, 0, len(programs)+len(harnessHelperShells())+1)
	for _, name := range programs {
		specs = append(specs, harnessWrapperSpec{
			path:        filepath.Join(srcDir, name),
			commandName: name,
		})
	}
	for _, name := range harnessHelperShells() {
		specs = append(specs, harnessWrapperSpec{
			path: filepath.Join(srcDir, name),
		})
	}
	specs = append(specs, harnessWrapperSpec{
		path:        filepath.Join(srcDir, "ginstall"),
		commandName: "install",
	})
	return specs
}

func writeHarnessLauncher(workDir, gbashBin string) error {
	hookDir := harnessDir(workDir)
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return err
	}
	data, err := renderLauncherScript(gbashBin)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(hookDir, "gbash"), data, 0o755)
}

func writeHarnessRelinkScript(workDir string) error {
	hookDir := harnessDir(workDir)
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return err
	}
	data, err := renderRelinkScript()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(hookDir, "relink.sh"), data, 0o755)
}

func patchHarnessTestsEnvironment(workDir string) error {
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

func patchHarnessInitSetup(workDir string) error {
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
