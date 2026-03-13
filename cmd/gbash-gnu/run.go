package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type executionEnv struct {
	cacheDir   string
	gbashBin   string
	resultsDir string
	workDir    string
}

type executionPlan struct {
	runTargets    []attributedUtility
	explicitTests []string
	allTests      []string
	globalSkipped []string
	configShell   string
	utilsFiltered bool
}

type utilityRunExecution struct {
	results      []utilityResult
	overall      testSummary
	suiteResults []testResult
	extraResults []testResult
	hadFailure   bool
}

func run(ctx context.Context, mf *manifest, opts *options) error {
	makeBin, err := findMake()
	if err != nil {
		return err
	}
	if err := requireTool("perl"); err != nil {
		return err
	}
	if err := requireCC(); err != nil {
		return err
	}

	cacheDir, err := filepath.Abs(opts.cacheDir)
	if err != nil {
		return err
	}
	if opts.setupOnly {
		sourceDir, err := ensureSourceCache(ctx, mf, cacheDir)
		if err != nil {
			return err
		}
		fmt.Printf("GNU coreutils %s prepared at %s\n", mf.GNUVersion, sourceDir)
		return nil
	}
	if strings.TrimSpace(opts.writePreparedBuildArchive) != "" {
		return writePreparedBuildArchive(ctx, mf, opts, makeBin, cacheDir)
	}

	env, err := prepareExecutionEnvironment(ctx, mf, opts, makeBin, cacheDir)
	if err != nil {
		return err
	}
	if !opts.keepWorkdir {
		defer func() { _ = os.RemoveAll(env.workDir) }()
	}

	plan, err := prepareExecutionPlan(ctx, mf, env, opts)
	if err != nil {
		return err
	}

	summary, hadFailure, err := executeCompatibilityRun(ctx, makeBin, mf, env, plan)
	if err != nil {
		return err
	}
	summary.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	summary.UtilitySummary = summarizeUtilityTotals(summary.Utilities)

	summaryPath := filepath.Join(env.resultsDir, "summary.json")
	if err := writeJSON(summaryPath, summary); err != nil {
		return err
	}
	fmt.Printf("results: %s\n", env.resultsDir)
	if hadFailure {
		return fmt.Errorf("GNU compatibility run failed")
	}
	return nil
}

func writePreparedBuildArchive(ctx context.Context, mf *manifest, opts *options, makeBin, cacheDir string) error {
	archivePath, err := filepath.Abs(opts.writePreparedBuildArchive)
	if err != nil {
		return err
	}
	sourceDir, err := ensureSourceCache(ctx, mf, cacheDir)
	if err != nil {
		return err
	}
	if err := buildPreparedBuildArchive(ctx, makeBin, cacheDir, mf.GNUVersion, sourceDir, archivePath, opts.keepWorkdir); err != nil {
		return err
	}
	fmt.Printf("prepared GNU build archive: %s\n", archivePath)
	return nil
}

func prepareExecutionEnvironment(ctx context.Context, mf *manifest, opts *options, makeBin, cacheDir string) (*executionEnv, error) {
	gbashBin, err := filepath.Abs(opts.gbashBin)
	if err != nil {
		return nil, err
	}
	if err := ensureExecutable(gbashBin); err != nil {
		return nil, err
	}

	resultsDir, err := prepareResultsDir(cacheDir, opts.resultsDir)
	if err != nil {
		return nil, fmt.Errorf("create results dir: %w", err)
	}

	preparedArchivePath := strings.TrimSpace(opts.preparedBuildArchive)
	if preparedArchivePath != "" {
		preparedArchivePath, err = filepath.Abs(preparedArchivePath)
		if err != nil {
			return nil, err
		}
	}

	workDir, err := prepareExecutionWorkDir(ctx, mf, makeBin, cacheDir, preparedArchivePath)
	if err != nil {
		return nil, err
	}

	return &executionEnv{
		cacheDir:   cacheDir,
		gbashBin:   gbashBin,
		resultsDir: resultsDir,
		workDir:    workDir,
	}, nil
}

func prepareExecutionWorkDir(ctx context.Context, mf *manifest, makeBin, cacheDir, preparedArchivePath string) (string, error) {
	var workDir string
	var err error
	if preparedArchivePath != "" {
		workDir, err = prepareWorkDirFromPreparedArchive(ctx, cacheDir, mf.GNUVersion, preparedArchivePath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "gbash-gnu: prepared build archive unavailable (%v); falling back to full build\n", err)
		}
	}
	if workDir != "" {
		return workDir, nil
	}
	if builtDir, err := findPreviousBuild(cacheDir, mf.GNUVersion); err == nil {
		workDir, err = prepareWorkDir(cacheDir, mf.GNUVersion, builtDir)
		if err != nil {
			return "", err
		}
		if err := relocatePreparedBuild(ctx, workDir); err != nil {
			return "", err
		}
		return workDir, nil
	}

	sourceDir, err := ensureSourceCache(ctx, mf, cacheDir)
	if err != nil {
		return "", err
	}
	workDir, err = prepareWorkDir(cacheDir, mf.GNUVersion, sourceDir)
	if err != nil {
		return "", err
	}
	if err := configureAndBuild(ctx, makeBin, workDir); err != nil {
		_ = os.RemoveAll(workDir)
		return "", err
	}
	return workDir, nil
}

func prepareExecutionPlan(ctx context.Context, mf *manifest, env *executionEnv, opts *options) (*executionPlan, error) {
	programs, err := listGNUPrograms(ctx, env.workDir)
	if err != nil {
		return nil, err
	}
	selectedUtilities, err := selectUtilities(programs, mf, opts.utils)
	if err != nil {
		return nil, err
	}
	runTargets := append([]attributedUtility(nil), selectedUtilities...)
	explicitTests := parseList(opts.tests)
	if err := prepareProgramDir(env.workDir, env.gbashBin, programs); err != nil {
		return nil, err
	}
	configShell, err := compatConfigShellPath(env.workDir)
	if err != nil {
		return nil, err
	}
	if err := disableCheckRebuild(env.workDir); err != nil {
		return nil, err
	}
	allTests, globalSkipped, err := discoverRunnableTests(env.workDir, mf.SkipPatterns, explicitTests)
	if err != nil {
		return nil, err
	}

	return &executionPlan{
		runTargets:    runTargets,
		explicitTests: explicitTests,
		allTests:      allTests,
		globalSkipped: globalSkipped,
		configShell:   configShell,
		utilsFiltered: strings.TrimSpace(opts.utils) != "",
	}, nil
}

func executeCompatibilityRun(ctx context.Context, makeBin string, mf *manifest, env *executionEnv, plan *executionPlan) (runSummary, bool, error) {
	summary := runSummary{
		GNUVersion: mf.GNUVersion,
		WorkDir:    env.workDir,
		ResultsDir: env.resultsDir,
	}
	utilityRuns, _, err := resolveUtilityRuns(env.workDir, plan.runTargets, mf.SkipPatterns, plan.explicitTests)
	if err != nil {
		return runSummary{}, false, err
	}
	selectedTests := append([]string(nil), plan.allTests...)
	filteredEntries := parseSkippedEntries(plan.globalSkipped)
	filteredSkipTotal := len(filteredEntries)
	if plan.utilsFiltered || len(plan.explicitTests) != 0 {
		selectedTests = combinedTestsForRuns(utilityRuns)
		filteredEntries = combinedSkippedEntriesForRuns(utilityRuns)
		filteredSkipTotal = len(filteredEntries)
	}

	execution, err := executeUtilityRuns(ctx, makeBin, env, plan.configShell, utilityRuns, selectedTests, filteredSkipTotal)
	if err != nil {
		return runSummary{}, false, err
	}
	summary.Utilities = append(summary.Utilities, execution.results...)
	summary.Overall = execution.overall
	summary.Suite, summary.Categories, summary.Commands, summary.Coverage, summary.ExtraResults = buildCoverageArtifacts(selectedTests, filteredEntries, execution.suiteResults, execution.extraResults, utilityRuns, mf)
	return summary, execution.hadFailure, nil
}

func executeUtilityRuns(ctx context.Context, makeBin string, env *executionEnv, configShell string, runs []utilityRun, allTests []string, filteredSkipTotal int) (utilityRunExecution, error) {
	if len(runs) == 1 && runs[0].Utility.Name == "explicit-tests" {
		return executeUtilityRunsIndividually(ctx, makeBin, env, configShell, runs)
	}
	return executeUtilityRunsBatched(ctx, makeBin, env, configShell, runs, allTests, filteredSkipTotal)
}

func executeUtilityRunsIndividually(ctx context.Context, makeBin string, env *executionEnv, configShell string, runs []utilityRun) (utilityRunExecution, error) {
	results := make([]utilityResult, 0, len(runs))
	hadFailure := false
	var overall testSummary
	suiteByName := make(map[string]testResult)
	extraByName := make(map[string]testResult)
	for _, run := range runs {
		result := utilityResult{
			Name:    run.Utility.Name,
			Tests:   run.Tests,
			Skipped: run.Skipped,
			Summary: summarizeTestResults(nil, len(run.Skipped), 0),
		}
		if len(run.Tests) == 0 {
			results = append(results, result)
			continue
		}

		logFile := run.Utility.Name + ".log"
		logPath := filepath.Join(env.resultsDir, logFile)
		makeCheckResult, err := runMakeCheck(ctx, makeBin, env.workDir, configShell, run.Tests, logPath)
		if err != nil {
			return utilityRunExecution{}, err
		}
		result.TestResults, result.ExtraResults = parseReportedTestResults(makeCheckResult.Output, run.Tests)
		mergeNamedResults(suiteByName, result.TestResults)
		mergeNamedResults(extraByName, result.ExtraResults)
		result.Summary = summarizeTestResults(result.TestResults, len(run.Skipped), len(result.ExtraResults))
		result.LogFile = logFile
		result.LogPath = logPath
		result.ExitCode = makeCheckResult.ExitCode
		result.Passed = makeCheckResult.ExitCode == 0
		if makeCheckResult.ExitCode != 0 {
			hadFailure = true
		}
		results = append(results, result)
		fmt.Printf("%s: %d tests, exit=%d, pass=%s\n", run.Utility.Name, len(run.Tests), makeCheckResult.ExitCode, formatPercent(result.Summary.PassPctSelected))
	}
	overall = summarizeOverall(results)
	return utilityRunExecution{
		results:      results,
		overall:      overall,
		suiteResults: sortedNamedResults(suiteByName),
		extraResults: sortedNamedResults(extraByName),
		hadFailure:   hadFailure,
	}, nil
}

func executeUtilityRunsBatched(ctx context.Context, makeBin string, env *executionEnv, configShell string, runs []utilityRun, allTests []string, filteredSkipTotal int) (utilityRunExecution, error) {
	if len(allTests) == 0 {
		results := make([]utilityResult, 0, len(runs))
		for _, run := range runs {
			result := utilityResult{
				Name:    run.Utility.Name,
				Tests:   run.Tests,
				Skipped: run.Skipped,
				Summary: summarizeTestResults(nil, len(run.Skipped), 0),
			}
			results = append(results, result)
		}
		return utilityRunExecution{
			results: results,
			overall: testSummary{FilteredSkipTotal: filteredSkipTotal},
		}, nil
	}

	logFile := "compat.log"
	logPath := filepath.Join(env.resultsDir, logFile)
	makeCheckResult, err := runMakeCheck(ctx, makeBin, env.workDir, configShell, allTests, logPath)
	if err != nil {
		return utilityRunExecution{}, err
	}
	combinedResults, combinedExtras := parseReportedTestResults(makeCheckResult.Output, allTests)

	results, overall := buildBatchedUtilityResults(runs, allTests, filteredSkipTotal, makeCheckResult, logFile, logPath)
	hadFailure := !utilityPassed(&overall)
	for i := range results {
		result := &results[i]
		fmt.Printf("%s: %d tests, exit=%d, pass=%s\n", result.Name, len(result.Tests), result.ExitCode, formatPercent(result.Summary.PassPctSelected))
	}
	return utilityRunExecution{
		results:      results,
		overall:      overall,
		suiteResults: combinedResults,
		extraResults: combinedExtras,
		hadFailure:   hadFailure,
	}, nil
}

func combinedTestsForRuns(runs []utilityRun) []string {
	combined := make([]string, 0)
	seen := make(map[string]struct{})
	for _, run := range runs {
		for _, test := range run.Tests {
			if _, ok := seen[test]; ok {
				continue
			}
			seen[test] = struct{}{}
			combined = append(combined, test)
		}
	}
	return combined
}

func buildBatchedUtilityResults(runs []utilityRun, allTests []string, filteredSkipTotal int, makeCheckResult makeCheckResult, logFile, logPath string) ([]utilityResult, testSummary) {
	combinedResults, combinedExtras := parseReportedTestResults(makeCheckResult.Output, allTests)
	resultByName := make(map[string]testResult, len(combinedResults))
	for _, result := range combinedResults {
		resultByName[result.Name] = result
	}
	overall := summarizeTestResults(combinedResults, filteredSkipTotal, len(combinedExtras))

	results := make([]utilityResult, 0, len(runs))
	for _, run := range runs {
		result := utilityResult{
			Name:    run.Utility.Name,
			Tests:   run.Tests,
			Skipped: run.Skipped,
			Summary: summarizeTestResults(nil, len(run.Skipped), 0),
		}
		if len(run.Tests) == 0 {
			results = append(results, result)
			continue
		}

		testResults := make([]testResult, 0, len(run.Tests))
		for _, testName := range run.Tests {
			canonicalName := filepath.ToSlash(testName)
			if mapped, ok := resultByName[canonicalName]; ok {
				testResults = append(testResults, mapped)
				continue
			}
			testResults = append(testResults, testResult{Name: canonicalName, Status: "unreported"})
		}

		extraResults := extraResultsForUtility(run.Utility, combinedExtras)
		result.TestResults = testResults
		result.ExtraResults = extraResults
		result.Summary = summarizeTestResults(result.TestResults, len(run.Skipped), len(result.ExtraResults))
		result.LogFile = logFile
		result.LogPath = logPath
		result.ExitCode = utilityExitCode(&result.Summary, makeCheckResult.ExitCode)
		result.Passed = utilityPassed(&result.Summary)
		results = append(results, result)
	}
	return results, overall
}

func extraResultsForUtility(utility attributedUtility, extras []testResult) []testResult {
	if len(extras) == 0 {
		return nil
	}
	filtered := make([]testResult, 0)
	for _, extra := range extras {
		if utilityMatchesTestPath(utility, extra.Name) {
			filtered = append(filtered, extra)
		}
	}
	return filtered
}

func utilityMatchesTestPath(utility attributedUtility, rel string) bool {
	for _, pattern := range utility.Patterns {
		if matched, err := filepath.Match(pattern, rel); err == nil && matched {
			return true
		}
	}
	return false
}

func utilityPassed(summary *testSummary) bool {
	return summary.Fail == 0 && summary.XPass == 0 && summary.Error == 0 && summary.Unreported == 0
}

func utilityExitCode(summary *testSummary, batchExitCode int) int {
	if utilityPassed(summary) {
		return 0
	}
	if batchExitCode != 0 {
		return batchExitCode
	}
	return 1
}
