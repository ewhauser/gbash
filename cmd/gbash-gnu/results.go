package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

func parseReportedTestResults(logData []byte, selectedTests []string) (selectedResultsOut, extraResultsOut []testResult) {
	aliases := buildTestAliases(selectedTests)
	selectedResults := make(map[string]*testResult, len(selectedTests))
	extraResults := make(map[string]*testResult)

	for rawLine := range strings.SplitSeq(string(logData), "\n") {
		status, reportedName, ok := parseReportedTestStatusLine(rawLine)
		if !ok {
			continue
		}
		reportedName = filepath.ToSlash(reportedName)
		canonicalName := reportedName
		target := extraResults
		if mappedName, ok := aliases[reportedName]; ok {
			canonicalName = mappedName
			target = selectedResults
		}
		recordReportedTestStatus(target, canonicalName, reportedName, status)
	}

	results := make([]testResult, 0, len(selectedTests))
	for _, selectedTest := range selectedTests {
		canonicalName := filepath.ToSlash(selectedTest)
		if result, ok := selectedResults[canonicalName]; ok {
			results = append(results, *result)
			continue
		}
		results = append(results, testResult{Name: canonicalName, Status: "unreported"})
	}

	extraKeys := make([]string, 0, len(extraResults))
	for name := range extraResults {
		extraKeys = append(extraKeys, name)
	}
	sort.Strings(extraKeys)

	extras := make([]testResult, 0, len(extraKeys))
	for _, name := range extraKeys {
		extras = append(extras, *extraResults[name])
	}
	return results, extras
}

func buildTestAliases(selectedTests []string) map[string]string {
	candidates := make(map[string][]string, len(selectedTests)*2)
	for _, selectedTest := range selectedTests {
		canonicalName := filepath.ToSlash(selectedTest)
		candidates[canonicalName] = append(candidates[canonicalName], canonicalName)
		switch ext := filepath.Ext(canonicalName); ext {
		case ".pl", ".sh", ".xpl":
			alias := strings.TrimSuffix(canonicalName, ext)
			candidates[alias] = append(candidates[alias], canonicalName)
		}
	}

	aliases := make(map[string]string, len(candidates))
	for alias, names := range candidates {
		if len(names) == 1 {
			aliases[alias] = names[0]
		}
	}
	return aliases
}

func parseReportedTestStatusLine(line string) (status, name string, ok bool) {
	line = strings.TrimSpace(line)
	for prefix, statusName := range map[string]string{
		"PASS:":  "pass",
		"FAIL:":  "fail",
		"SKIP:":  "skip",
		"XFAIL:": "xfail",
		"XPASS:": "xpass",
		"ERROR:": "error",
	} {
		if strings.HasPrefix(line, prefix) {
			testName := strings.TrimSpace(line[len(prefix):])
			if testName == "" {
				return "", "", false
			}
			return statusName, testName, true
		}
	}
	return "", "", false
}

func recordReportedTestStatus(results map[string]*testResult, canonicalName, reportedName, status string) {
	existing := results[canonicalName]
	if existing == nil {
		results[canonicalName] = &testResult{
			Name:       canonicalName,
			Status:     status,
			ReportedAs: []string{reportedName},
		}
		return
	}
	if testStatusPrecedence(status) > testStatusPrecedence(existing.Status) {
		existing.Status = status
	}
	if !containsString(existing.ReportedAs, reportedName) {
		existing.ReportedAs = append(existing.ReportedAs, reportedName)
	}
}

func testStatusPrecedence(status string) int {
	switch status {
	case "error":
		return 6
	case "xpass":
		return 5
	case "fail":
		return 4
	case "xfail":
		return 3
	case "skip":
		return 2
	case "pass":
		return 1
	default:
		return 0
	}
}

func summarizeTestResults(results []testResult, filteredSkipTotal, reportedExtraTotal int) testSummary {
	summary := testSummary{
		SelectedTotal:      len(results),
		FilteredSkipTotal:  filteredSkipTotal,
		ReportedExtraTotal: reportedExtraTotal,
	}
	for _, result := range results {
		switch result.Status {
		case "pass":
			summary.Pass++
		case "fail":
			summary.Fail++
		case "skip":
			summary.Skip++
		case "xfail":
			summary.XFail++
		case "xpass":
			summary.XPass++
		case "error":
			summary.Error++
		default:
			summary.Unreported++
		}
	}
	summary.RunnableTotal = summary.Pass + summary.Fail + summary.XFail + summary.XPass + summary.Error
	summary.PassPctSelected = percentage(summary.Pass, summary.SelectedTotal)
	summary.PassPctRunnable = percentage(summary.Pass, summary.RunnableTotal)
	return summary
}

func summarizeOverall(results []utilityResult) testSummary {
	overall := testSummary{}
	for i := range results {
		utility := &results[i]
		overall.SelectedTotal += utility.Summary.SelectedTotal
		overall.FilteredSkipTotal += utility.Summary.FilteredSkipTotal
		overall.ReportedExtraTotal += utility.Summary.ReportedExtraTotal
		overall.Pass += utility.Summary.Pass
		overall.Fail += utility.Summary.Fail
		overall.Skip += utility.Summary.Skip
		overall.XFail += utility.Summary.XFail
		overall.XPass += utility.Summary.XPass
		overall.Error += utility.Summary.Error
		overall.Unreported += utility.Summary.Unreported
		overall.RunnableTotal += utility.Summary.RunnableTotal
	}
	overall.PassPctSelected = percentage(overall.Pass, overall.SelectedTotal)
	overall.PassPctRunnable = percentage(overall.Pass, overall.RunnableTotal)
	return overall
}

func summarizeUtilityTotals(results []utilityResult) utilityTotals {
	totals := utilityTotals{}
	for i := range results {
		result := &results[i]
		if result.Inactive {
			continue
		}
		totals.Total++
		if result.Summary.RunnableTotal == 0 {
			totals.NoRunnableTests++
			continue
		}
		if result.Passed {
			totals.Passed++
			continue
		}
		totals.Failed++
	}
	totals.PassPctTotal = percentage(totals.Passed, totals.Total)
	totals.PassPctRunnable = percentage(totals.Passed, totals.Passed+totals.Failed)
	return totals
}

func completeUtilityResults(results []utilityResult, programs []string, manifestUtilities, selectedUtilities []utilityManifest, supportedSet map[string]struct{}) []utilityResult {
	activeByName := make(map[string]utilityResult, len(results))
	for i := range results {
		result := results[i]
		activeByName[result.Name] = result
	}
	manifestSet := make(map[string]struct{}, len(manifestUtilities))
	for _, utility := range manifestUtilities {
		manifestSet[utility.Name] = struct{}{}
	}
	selectedSet := make(map[string]struct{}, len(selectedUtilities))
	for _, utility := range selectedUtilities {
		selectedSet[utility.Name] = struct{}{}
	}

	out := make([]utilityResult, 0, len(programs))
	for _, name := range programs {
		if result, ok := activeByName[name]; ok {
			out = append(out, result)
			delete(activeByName, name)
			continue
		}
		out = append(out, utilityResult{
			Name:     name,
			Inactive: true,
			Reason:   inactiveUtilityReason(name, manifestSet, selectedSet, supportedSet),
		})
	}
	for i := range results {
		result := results[i]
		if _, ok := activeByName[result.Name]; ok {
			out = append(out, result)
			delete(activeByName, result.Name)
		}
	}
	return out
}

func inactiveUtilityReason(name string, manifestSet, selectedSet, supportedSet map[string]struct{}) string {
	if _, ok := manifestSet[name]; ok {
		if _, ok := selectedSet[name]; ok {
			return ""
		}
		return "not selected in this run"
	}
	if _, ok := supportedSet[name]; ok {
		return "implemented in gbash, but not included in the compatibility manifest"
	}
	return "not currently covered by the compatibility manifest"
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func formatPercent(value float64) string {
	if value == 0 {
		return "0%"
	}
	if value == math.Trunc(value) {
		return fmt.Sprintf("%.0f%%", value)
	}
	formatted := fmt.Sprintf("%.2f", value)
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	return formatted + "%"
}

func percentage(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return math.Round((float64(numerator)/float64(denominator))*10000) / 100
}

func containsString(items []string, needle string) bool {
	return slices.Contains(items, needle)
}

func prepareResultsDir(cacheDir, explicitDir string) (string, error) {
	if strings.TrimSpace(explicitDir) == "" {
		root := filepath.Join(cacheDir, "results")
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", err
		}
		return os.MkdirTemp(root, "run-")
	}

	resultsDir, err := filepath.Abs(explicitDir)
	if err != nil {
		return "", err
	}
	if filepath.Clean(resultsDir) == string(os.PathSeparator) {
		return "", fmt.Errorf("refusing to use filesystem root as results dir")
	}
	info, err := os.Stat(resultsDir)
	switch {
	case err == nil && !info.IsDir():
		return "", fmt.Errorf("results dir %s exists and is not a directory", resultsDir)
	case err == nil:
		entries, err := os.ReadDir(resultsDir)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(resultsDir, entry.Name())); err != nil {
				return "", err
			}
		}
	case errorsIsNotExist(err):
		if err := os.MkdirAll(resultsDir, 0o755); err != nil {
			return "", err
		}
	default:
		return "", err
	}
	return resultsDir, nil
}
