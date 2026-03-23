package gbasheval

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strconv"
	"strings"

	gbfs "github.com/ewhauser/gbash/fs"
)

type ScoreResult struct {
	Check  string  `json:"check"`
	Passed bool    `json:"passed"`
	Detail string  `json:"detail"`
	Weight float64 `json:"weight"`
}

type TaskScore struct {
	TaskID   string        `json:"task_id"`
	Results  []ScoreResult `json:"results"`
	Score    float64       `json:"score"`
	MaxScore float64       `json:"max_score"`
}

func (s TaskScore) AllPassed() bool {
	for _, result := range s.Results {
		if !result.Passed {
			return false
		}
	}
	return true
}

func scoreTask(taskID string, trace agentTrace, fsys gbfs.FileSystem, expectations []Expectation) TaskScore {
	results := make([]ScoreResult, 0, len(expectations))
	for _, exp := range expectations {
		results = append(results, evaluateCheck(trace, fsys, exp))
	}

	var maxScore float64
	var score float64
	for _, result := range results {
		maxScore += result.Weight
		if result.Passed {
			score += result.Weight
		}
	}

	return TaskScore{
		TaskID:   taskID,
		Results:  results,
		Score:    score,
		MaxScore: maxScore,
	}
}

func evaluateCheck(trace agentTrace, fsys gbfs.FileSystem, exp Expectation) ScoreResult {
	checkType, checkValue, _ := strings.Cut(exp.Check, ":")
	switch checkType {
	case "exit_code":
		expected, _ := strconv.Atoi(checkValue)
		actual := -1
		if trace.LastToolResponse != nil {
			actual = trace.LastToolResponse.ExitCode
		}
		return ScoreResult{
			Check:  exp.Check,
			Passed: actual == expected,
			Detail: fmt.Sprintf("expected %d, got %d", expected, actual),
			Weight: exp.Weight,
		}
	case "stdout_contains":
		for _, call := range trace.ToolCalls {
			if strings.Contains(call.Stdout, checkValue) {
				return ScoreResult{Check: exp.Check, Passed: true, Detail: "found", Weight: exp.Weight}
			}
		}
		return ScoreResult{
			Check:  exp.Check,
			Passed: false,
			Detail: fmt.Sprintf("%q not found in any tool output", checkValue),
			Weight: exp.Weight,
		}
	case "stdout_regex":
		re, err := regexp.Compile(checkValue)
		if err != nil {
			return ScoreResult{Check: exp.Check, Passed: false, Detail: fmt.Sprintf("invalid regex: %v", err), Weight: exp.Weight}
		}
		for _, call := range trace.ToolCalls {
			if re.MatchString(call.Stdout) {
				return ScoreResult{Check: exp.Check, Passed: true, Detail: "matched", Weight: exp.Weight}
			}
		}
		return ScoreResult{
			Check:  exp.Check,
			Passed: false,
			Detail: fmt.Sprintf("pattern %q not matched", checkValue),
			Weight: exp.Weight,
		}
	case "stderr_empty":
		for _, call := range trace.ToolCalls {
			if call.Stderr != "" {
				detail := call.Stderr
				if len(detail) > 100 {
					detail = detail[:100]
				}
				return ScoreResult{Check: exp.Check, Passed: false, Detail: fmt.Sprintf("stderr: %s", detail), Weight: exp.Weight}
			}
		}
		return ScoreResult{Check: exp.Check, Passed: true, Detail: "all stderr empty", Weight: exp.Weight}
	case "file_exists":
		err := statPath(fsys, checkValue)
		return ScoreResult{Check: exp.Check, Passed: err == nil, Detail: existsDetail(err), Weight: exp.Weight}
	case "dir_exists":
		info, err := statPathInfo(fsys, checkValue)
		passed := err == nil && info.IsDir()
		detail := "directory not found"
		if passed {
			detail = "directory exists"
		}
		return ScoreResult{Check: exp.Check, Passed: passed, Detail: detail, Weight: exp.Weight}
	case "file_contains":
		path, text, ok := strings.Cut(checkValue, ":")
		if !ok {
			return ScoreResult{Check: exp.Check, Passed: false, Detail: "invalid format, expected file_contains:/path:text", Weight: exp.Weight}
		}
		data, err := readFile(context.Background(), fsys, path)
		if err != nil {
			return ScoreResult{Check: exp.Check, Passed: false, Detail: err.Error(), Weight: exp.Weight}
		}
		passed := strings.Contains(string(data), text)
		detail := "found"
		if !passed {
			detail = fmt.Sprintf("%q not found in %s", text, path)
		}
		return ScoreResult{Check: exp.Check, Passed: passed, Detail: detail, Weight: exp.Weight}
	case "llm_judge":
		return ScoreResult{Check: exp.Check, Passed: true, Detail: "llm_judge not implemented (stub, weight=0)", Weight: 0}
	default:
		return ScoreResult{Check: exp.Check, Passed: false, Detail: fmt.Sprintf("unknown check type: %s", checkType), Weight: exp.Weight}
	}
}

func readFile(ctx context.Context, fsys gbfs.FileSystem, name string) ([]byte, error) {
	if fsys == nil {
		return nil, errors.New("filesystem is nil")
	}
	file, err := fsys.Open(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

func statPath(fsys gbfs.FileSystem, name string) error {
	_, err := statPathInfo(fsys, name)
	return err
}

func statPathInfo(fsys gbfs.FileSystem, name string) (fs.FileInfo, error) {
	if fsys == nil {
		return nil, errors.New("filesystem is nil")
	}
	return fsys.Stat(context.Background(), name)
}

func existsDetail(err error) string {
	if err == nil {
		return "exists"
	}
	if errors.Is(err, fs.ErrNotExist) {
		return "not found"
	}
	return err.Error()
}
