package gbasheval

import (
	"context"
	"fmt"
	"io"
)

func Run(ctx context.Context, cfg RunConfig, stdout, stderr io.Writer) error {
	provider, err := createProvider(cfg.ProviderName, cfg.Model, stderr)
	if err != nil {
		return err
	}
	if cfg.EvalType == "scripting-tool" {
		return runScriptingEval(ctx, cfg, provider, stdout)
	}
	return runBashEval(ctx, cfg, provider, stdout)
}

func runBashEval(ctx context.Context, cfg RunConfig, provider Provider, stdout io.Writer) error {
	tasks, err := loadDataset(cfg.DatasetPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Running %d tasks with %s/%s  (max_turns=%d)\n\n", len(tasks), cfg.ProviderName, cfg.Model, cfg.MaxTurns)

	results := make([]EvalResult, 0, len(tasks))
	for i, task := range tasks {
		fmt.Fprintf(stdout, "[%d/%d] %s - %s\n", i+1, len(tasks), task.ID, task.Description)

		trace, fsys, err := runAgentLoop(ctx, provider, task, cfg.MaxTurns)
		if err != nil {
			fmt.Fprintf(stdout, "  ERROR: %v\n\n", err)
			continue
		}
		score := scoreTask(task.ID, trace, fsys, task.Expectations)
		for _, result := range score.Results {
			icon := "FAIL"
			if result.Passed {
				icon = "PASS"
			}
			fmt.Fprintf(stdout, "  [%s] %s - %s\n", icon, result.Check, result.Detail)
		}
		callsOK := 0
		for _, call := range trace.ToolCalls {
			if call.ExitCode == 0 {
				callsOK++
			}
		}
		fmt.Fprintf(stdout, "  Score: %.0f/%.0f | Turns: %d | Calls: %d (%d ok, %d err) | Tokens: %din/%dout | %.1fs\n\n",
			score.Score, score.MaxScore, trace.Turns, trace.ToolCallCount, callsOK, trace.ToolCallCount-callsOK,
			trace.TotalInputTokens, trace.TotalOutputTokens, float64(trace.DurationMS)/1000,
		)
		results = append(results, EvalResult{Task: task, Trace: trace, Score: score})
	}

	report := buildEvalReport(cfg.ProviderName, cfg.Model, "bash", cfg.MaxTurns, results)
	printEvalTerminalReport(stdout, report)
	if cfg.Save {
		if err := saveEvalReport(report, cfg.OutputDir, cfg.Moniker, stdout); err != nil {
			return err
		}
	}
	return nil
}
