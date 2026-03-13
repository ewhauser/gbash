package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func selectUtilities(programs []string, mf *manifest, raw string) ([]attributedUtility, error) {
	utilities := discoverAttributedUtilities(programs, mf)
	if strings.TrimSpace(raw) == "" {
		return utilities, nil
	}

	allowed := make(map[string]attributedUtility, len(utilities))
	for _, utility := range utilities {
		allowed[utility.Name] = utility
	}

	selected := parseList(raw)
	out := make([]attributedUtility, 0, len(selected))
	for _, name := range selected {
		utility, ok := allowed[name]
		if !ok {
			return nil, fmt.Errorf("unknown utility %q", name)
		}
		out = append(out, utility)
	}
	return out, nil
}

func discoverAttributedUtilities(programs []string, mf *manifest) []attributedUtility {
	aliases := manifestUtilityAliases(mf)
	overrideByName := make(map[string]utilityAttribution, len(mf.UtilityOverrides))
	for _, override := range mf.UtilityOverrides {
		overrideByName[override.Name] = override
	}

	utilities := make([]attributedUtility, 0, len(programs)+len(mf.UtilityOverrides))
	seen := make(map[string]struct{}, len(programs)+len(mf.UtilityOverrides))
	for _, program := range programs {
		name := utilityDisplayName(program, aliases)
		override, hasOverride := overrideByName[name]
		utility := attributedUtility{
			Name:     name,
			Patterns: []string{filepath.ToSlash(filepath.Join("tests", program, "*"))},
		}
		if hasOverride {
			utility.Patterns = append(utility.Patterns, override.Patterns...)
			utility.Skips = append(utility.Skips, override.Skips...)
		}
		utility.Patterns = uniqueSortedStrings(utility.Patterns)
		utilities = append(utilities, utility)
		seen[name] = struct{}{}
	}

	for _, override := range mf.UtilityOverrides {
		if _, ok := seen[override.Name]; ok {
			continue
		}
		utilities = append(utilities, attributedUtility{
			Name:     override.Name,
			Patterns: uniqueSortedStrings(append([]string(nil), override.Patterns...)),
			Skips:    uniqueSortedStrings(append([]string(nil), override.Skips...)),
		})
	}

	sort.Slice(utilities, func(i, j int) bool {
		return utilities[i].Name < utilities[j].Name
	})
	return utilities
}

func manifestUtilityAliases(mf *manifest) map[string]string {
	aliases := make(map[string]string, len(mf.UtilityDisplayNames))
	for _, alias := range mf.UtilityDisplayNames {
		aliases[alias.Name] = alias.Alias
	}
	return aliases
}

func utilityDisplayName(program string, aliases map[string]string) string {
	if alias, ok := aliases[program]; ok && strings.TrimSpace(alias) != "" {
		return alias
	}
	return program
}

func discoverRunnableTests(workDir string, globalSkips []skipPattern, explicitTests []string) (testsOut, skippedOut []string, err error) {
	if len(explicitTests) != 0 {
		filtered := make([]string, 0, len(explicitTests))
		skipped := make([]string, 0)
		for _, test := range explicitTests {
			rel := filepath.ToSlash(test)
			if skip, reason, err := shouldSkipTest(rel, filepath.Join(workDir, test), globalSkips, nil); err != nil {
				return nil, nil, err
			} else if skip {
				skipped = append(skipped, rel+": "+reason)
			} else {
				filtered = append(filtered, rel)
			}
		}
		return uniqueSortedStrings(filtered), uniqueSortedStrings(skipped), nil
	}

	tests := make([]string, 0)
	skipped := make([]string, 0)
	testsRoot := filepath.Join(workDir, "tests")
	err = filepath.Walk(testsRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "tests/") {
			return nil
		}
		if !isRunnableTestFile(rel, info) {
			return nil
		}
		if skip, reason, err := shouldSkipTest(rel, path, globalSkips, nil); err != nil {
			return err
		} else if skip {
			skipped = append(skipped, rel+": "+reason)
		} else {
			tests = append(tests, rel)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return uniqueSortedStrings(tests), uniqueSortedStrings(skipped), nil
}

func resolveUtilityRuns(workDir string, utilities []attributedUtility, globalSkips []skipPattern, explicitTests []string) ([]utilityRun, []string, error) {
	if len(explicitTests) != 0 {
		tests, skipped, err := discoverRunnableTests(workDir, globalSkips, explicitTests)
		if err != nil {
			return nil, nil, err
		}
		return []utilityRun{{
			Utility: attributedUtility{Name: "explicit-tests"},
			Tests:   tests,
			Skipped: skipped,
		}}, tests, nil
	}

	allTests, _, err := discoverRunnableTests(workDir, globalSkips, nil)
	if err != nil {
		return nil, nil, err
	}

	runs := make([]utilityRun, 0, len(utilities))
	for _, utility := range utilities {
		tests, skipped := attributeUtilityTests(allTests, utility)
		runs = append(runs, utilityRun{
			Utility: utility,
			Tests:   tests,
			Skipped: skipped,
		})
	}
	return runs, allTests, nil
}

func attributeUtilityTests(allTests []string, utility attributedUtility) (tests, skipped []string) {
	tests = make([]string, 0)
	skipped = make([]string, 0)
	for _, test := range allTests {
		if !utilityMatchesTestPath(utility, test) {
			continue
		}
		if skip, reason := shouldSkipUtilityTest(test, utility.Skips); skip {
			skipped = append(skipped, test+": "+reason)
			continue
		}
		tests = append(tests, test)
	}
	return uniqueSortedStrings(tests), uniqueSortedStrings(skipped)
}

func shouldSkipUtilityTest(rel string, utilitySkips []string) (skip bool, reason string) {
	for _, pattern := range utilitySkips {
		if matched, err := filepath.Match(pattern, rel); err == nil && matched {
			return true, "utility-specific skip"
		}
	}
	return false, ""
}

func isRunnableTestFile(rel string, info os.FileInfo) bool {
	switch filepath.Ext(rel) {
	case ".log", ".trs":
		return false
	case ".sh", ".pl", ".xpl":
		return true
	default:
		return info.Mode()&0o111 != 0
	}
}

func shouldSkipTest(rel, path string, globalSkips []skipPattern, utilitySkips []string) (skip bool, reason string, err error) {
	for _, skip := range globalSkips {
		if matched, err := filepath.Match(skip.Pattern, rel); err == nil && matched {
			return true, skip.Reason, nil
		}
	}
	for _, pattern := range utilitySkips {
		if matched, err := filepath.Match(pattern, rel); err == nil && matched {
			return true, "utility-specific skip", nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, "", err
	}
	contents := string(data)
	switch {
	case strings.Contains(contents, "require_controlling_input_terminal"):
		return true, "controlling TTY tests are skipped in v1", nil
	case strings.Contains(contents, "require_root_"):
		return true, "root-required tests are skipped in v1", nil
	case strings.Contains(contents, "require_selinux_"):
		return true, "SELinux tests are skipped in v1", nil
	case strings.Contains(rel, "help-version"):
		return true, "help/version tests are skipped in v1", nil
	default:
		return false, "", nil
	}
}

func runMakeCheck(ctx context.Context, makeBin, workDir, configShell string, tests []string, logPath string) (makeCheckResult, error) {
	args := []string{
		"check",
		"SUBDIRS=.",
		"VERBOSE=no",
		"RUN_EXPENSIVE_TESTS=yes",
		"RUN_VERY_EXPENSIVE_TESTS=yes",
		"srcdir=" + workDir,
		"TESTS=" + strings.Join(tests, " "),
	}
	cmd := exec.CommandContext(ctx, makeBin, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"CONFIG_SHELL="+configShell,
	)
	output, err := cmd.CombinedOutput()
	if writeErr := os.WriteFile(logPath, output, 0o644); writeErr != nil {
		return makeCheckResult{}, writeErr
	}
	if err == nil {
		return makeCheckResult{Output: output}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return makeCheckResult{ExitCode: exitErr.ExitCode(), Output: output}, nil
	}
	return makeCheckResult{}, err
}

func uniqueSortedStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
