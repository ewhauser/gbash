package gbasheval

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type EvalResult struct {
	Task  EvalTask   `json:"task"`
	Trace agentTrace `json:"trace"`
	Score TaskScore  `json:"score"`
}

type EvalReport struct {
	Provider  string       `json:"provider"`
	Model     string       `json:"model"`
	Mode      string       `json:"mode"`
	Timestamp string       `json:"timestamp"`
	MaxTurns  int          `json:"max_turns"`
	Results   []EvalResult `json:"results"`
	Summary   EvalSummary  `json:"summary"`
}

type EvalSummary struct {
	TotalTasks          int                        `json:"total_tasks"`
	TotalPassed         int                        `json:"total_passed"`
	TotalScore          float64                    `json:"total_score"`
	TotalMaxScore       float64                    `json:"total_max_score"`
	OverallRate         float64                    `json:"overall_rate"`
	TotalInputTokens    uint32                     `json:"total_input_tokens"`
	TotalOutputTokens   uint32                     `json:"total_output_tokens"`
	TotalTurns          int                        `json:"total_turns"`
	TotalToolCalls      int                        `json:"total_tool_calls"`
	ToolCallsOK         int                        `json:"tool_calls_ok"`
	ToolCallsError      int                        `json:"tool_calls_error"`
	ToolCallSuccessRate float64                    `json:"tool_call_success_rate"`
	TotalDurationMS     uint64                     `json:"total_duration_ms"`
	AverageTurnsPerTask float64                    `json:"avg_turns_per_task"`
	AverageCallsPerTask float64                    `json:"avg_tool_calls_per_task"`
	AverageDurationMS   float64                    `json:"avg_duration_ms"`
	ByCategory          map[string]CategorySummary `json:"by_category"`
}

type CategorySummary struct {
	Tasks    int     `json:"tasks"`
	Passed   int     `json:"passed"`
	Score    float64 `json:"score"`
	MaxScore float64 `json:"max_score"`
	Rate     float64 `json:"rate"`
}

func buildEvalReport(providerName, model, mode string, maxTurns int, results []EvalResult) EvalReport {
	summary := EvalSummary{
		TotalTasks: len(results),
		ByCategory: map[string]CategorySummary{},
	}
	for _, result := range results {
		if result.Score.AllPassed() {
			summary.TotalPassed++
		}
		summary.TotalScore += result.Score.Score
		summary.TotalMaxScore += result.Score.MaxScore
		summary.TotalInputTokens += result.Trace.TotalInputTokens
		summary.TotalOutputTokens += result.Trace.TotalOutputTokens
		summary.TotalTurns += result.Trace.Turns
		summary.TotalToolCalls += result.Trace.ToolCallCount
		summary.TotalDurationMS += result.Trace.DurationMS
		for _, call := range result.Trace.ToolCalls {
			if call.ExitCode == 0 {
				summary.ToolCallsOK++
			}
		}

		cat := summary.ByCategory[result.Task.Category]
		cat.Tasks++
		if result.Score.AllPassed() {
			cat.Passed++
		}
		cat.Score += result.Score.Score
		cat.MaxScore += result.Score.MaxScore
		summary.ByCategory[result.Task.Category] = cat
	}

	if summary.TotalMaxScore > 0 {
		summary.OverallRate = summary.TotalScore / summary.TotalMaxScore
	}
	summary.ToolCallsError = summary.TotalToolCalls - summary.ToolCallsOK
	if summary.TotalToolCalls > 0 {
		summary.ToolCallSuccessRate = float64(summary.ToolCallsOK) / float64(summary.TotalToolCalls)
	} else {
		summary.ToolCallSuccessRate = 1
	}
	if summary.TotalTasks > 0 {
		n := float64(summary.TotalTasks)
		summary.AverageTurnsPerTask = float64(summary.TotalTurns) / n
		summary.AverageCallsPerTask = float64(summary.TotalToolCalls) / n
		summary.AverageDurationMS = float64(summary.TotalDurationMS) / n
	}
	for key, value := range summary.ByCategory {
		if value.MaxScore > 0 {
			value.Rate = value.Score / value.MaxScore
		} else {
			value.Rate = 1
		}
		summary.ByCategory[key] = value
	}

	return EvalReport{
		Provider:  providerName,
		Model:     model,
		Mode:      mode,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		MaxTurns:  maxTurns,
		Results:   results,
		Summary:   summary,
	}
}

func printEvalTerminalReport(w io.Writer, report EvalReport) {
	if w == nil {
		w = io.Discard
	}
	fmt.Fprintf(w, "\n=== Eval Report: %s/%s (%s) ===\n\n", report.Provider, report.Model, report.Mode)
	for _, result := range report.Results {
		status := "FAIL"
		if result.Score.AllPassed() {
			status = "PASS"
		}
		fmt.Fprintf(w, "  [%s] %s (%s) - %.0f/%.0f\n", status, result.Task.ID, result.Task.Category, result.Score.Score, result.Score.MaxScore)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "--- Summary ---")
	fmt.Fprintf(w, "  Tasks: %d/%d passed\n", report.Summary.TotalPassed, report.Summary.TotalTasks)
	fmt.Fprintf(w, "  Score: %.1f/%.1f (%.0f%%)\n", report.Summary.TotalScore, report.Summary.TotalMaxScore, report.Summary.OverallRate*100)
	fmt.Fprintf(w, "  Turns: %d total, %.1f avg/task\n", report.Summary.TotalTurns, report.Summary.AverageTurnsPerTask)
	fmt.Fprintf(w, "  Tool calls: %d total, %.1f avg/task (%d ok, %d error, %.0f%% success)\n", report.Summary.TotalToolCalls, report.Summary.AverageCallsPerTask, report.Summary.ToolCallsOK, report.Summary.ToolCallsError, report.Summary.ToolCallSuccessRate*100)
	fmt.Fprintf(w, "  Tokens: %d input, %d output\n", report.Summary.TotalInputTokens, report.Summary.TotalOutputTokens)
	fmt.Fprintf(w, "  Duration: %.1fs total, %.1fs avg/task\n", float64(report.Summary.TotalDurationMS)/1000, report.Summary.AverageDurationMS/1000)

	keys := make([]string, 0, len(report.Summary.ByCategory))
	for key := range report.Summary.ByCategory {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "--- By Category ---")
	for _, key := range keys {
		cat := report.Summary.ByCategory[key]
		fmt.Fprintf(w, "  %-25s %d/%d tasks  %.0f%%\n", key, cat.Passed, cat.Tasks, cat.Rate*100)
	}
	fmt.Fprintln(w)
}

func saveEvalReport(report EvalReport, outputDir, moniker string, stdout io.Writer) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %q: %w", outputDir, err)
	}
	base := filepath.Join(outputDir, fmt.Sprintf("eval-%s-%s", moniker, time.Now().UTC().Format("2006-01-02-150405")))

	jsonPath := base + ".json"
	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report json: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0o644); err != nil {
		return fmt.Errorf("write report json: %w", err)
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "Saved JSON: %s\n", jsonPath)
	}

	mdPath := base + ".md"
	if err := os.WriteFile(mdPath, []byte(generateEvalMarkdown(report)), 0o644); err != nil {
		return fmt.Errorf("write report markdown: %w", err)
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "Saved Markdown: %s\n", mdPath)
	}
	return nil
}

func generateEvalMarkdown(report EvalReport) string {
	var out string
	out += fmt.Sprintf("# Eval Report: %s/%s\n\n", report.Provider, report.Model)
	out += fmt.Sprintf("- Mode: `%s`\n", report.Mode)
	out += fmt.Sprintf("- Timestamp: `%s`\n", report.Timestamp)
	out += fmt.Sprintf("- Max turns: `%d`\n\n", report.MaxTurns)

	out += "## Summary\n\n"
	out += fmt.Sprintf("- Tasks passed: `%d/%d`\n", report.Summary.TotalPassed, report.Summary.TotalTasks)
	out += fmt.Sprintf("- Score: `%.1f/%.1f` (`%.0f%%`)\n", report.Summary.TotalScore, report.Summary.TotalMaxScore, report.Summary.OverallRate*100)
	out += fmt.Sprintf("- Tool call success: `%d/%d` (`%.0f%%`)\n", report.Summary.ToolCallsOK, report.Summary.TotalToolCalls, report.Summary.ToolCallSuccessRate*100)
	out += fmt.Sprintf("- Tokens: `%d` input / `%d` output\n", report.Summary.TotalInputTokens, report.Summary.TotalOutputTokens)
	out += fmt.Sprintf("- Duration: `%.1fs`\n\n", float64(report.Summary.TotalDurationMS)/1000)

	out += "## By Category\n\n"
	out += "| Category | Passed | Tasks | Score |\n"
	out += "|---|---:|---:|---:|\n"
	keys := make([]string, 0, len(report.Summary.ByCategory))
	for key := range report.Summary.ByCategory {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cat := report.Summary.ByCategory[key]
		out += fmt.Sprintf("| %s | %d | %d | %.0f%% |\n", key, cat.Passed, cat.Tasks, cat.Rate*100)
	}

	out += "\n## Task Results\n\n"
	out += "| Task | Category | Status | Score | Turns | Calls |\n"
	out += "|---|---|---|---:|---:|---:|\n"
	for _, result := range report.Results {
		status := "FAIL"
		if result.Score.AllPassed() {
			status = "PASS"
		}
		out += fmt.Sprintf("| %s | %s | %s | %.0f/%.0f | %d | %d |\n", result.Task.ID, result.Task.Category, status, result.Score.Score, result.Score.MaxScore, result.Trace.Turns, result.Trace.ToolCallCount)
	}
	return out
}
