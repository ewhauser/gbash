package builtins

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ewhauser/gbash/internal/shellstate"
)

var validCompletionOptions = []string{
	"bashdefault",
	"default",
	"dirnames",
	"filenames",
	"noquote",
	"nosort",
	"nospace",
	"plusdirs",
}

func completionStateFromContext(ctx context.Context) *shellstate.CompletionState {
	if state := shellstate.CompletionStateFromContext(ctx); state != nil {
		return state
	}
	return shellstate.NewCompletionState()
}

func isValidCompletionOption(name string) bool {
	return slices.Contains(validCompletionOptions, strings.TrimSpace(name))
}

func mergeCompletionOptions(current, enable, disable []string) []string {
	order := make([]string, 0, len(current)+len(enable))
	seen := make(map[string]struct{}, len(current)+len(enable))
	for _, opt := range current {
		if _, ok := seen[opt]; ok {
			continue
		}
		seen[opt] = struct{}{}
		order = append(order, opt)
	}
	for _, opt := range enable {
		if _, ok := seen[opt]; ok {
			continue
		}
		seen[opt] = struct{}{}
		order = append(order, opt)
	}
	if len(order) == 0 {
		return nil
	}

	disabled := make(map[string]struct{}, len(disable))
	for _, opt := range disable {
		disabled[opt] = struct{}{}
	}

	out := make([]string, 0, len(order))
	for _, opt := range order {
		if _, ok := disabled[opt]; ok {
			continue
		}
		out = append(out, opt)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func quoteCompletionWordlist(wordlist string) string {
	if !strings.ContainsAny(wordlist, " '") {
		return wordlist
	}
	return "'" + strings.ReplaceAll(wordlist, "'", `'\''`) + "'"
}

func formatCompleteSpec(name string, spec *shellstate.CompletionSpec) string {
	var b strings.Builder
	b.WriteString("complete")
	for _, opt := range spec.Options {
		fmt.Fprintf(&b, " -o %s", opt)
	}
	for _, action := range spec.Actions {
		fmt.Fprintf(&b, " -A %s", action)
	}
	if spec.HasWordlist {
		fmt.Fprintf(&b, " -W %s", quoteCompletionWordlist(spec.Wordlist))
	}
	if spec.HasFunction {
		fmt.Fprintf(&b, " -F %s", spec.Function)
	}
	if spec.HasCommand {
		fmt.Fprintf(&b, " -C %s", spec.Command)
	}
	switch name {
	case shellstate.CompletionSpecDefaultKey:
		b.WriteString(" -D")
	case shellstate.CompletionSpecEmptyKey:
		b.WriteString(" -E")
	default:
		fmt.Fprintf(&b, " %s", name)
	}
	return b.String()
}

func isInternalCompletionSpec(name string) bool {
	switch name {
	case shellstate.CompletionSpecDefaultKey, shellstate.CompletionSpecEmptyKey:
		return true
	default:
		return false
	}
}
