package runtime

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/ewhauser/gbash/trace"
)

const redactedValue = "[REDACTED]"

var sensitiveQueryKeys = map[string]struct{}{
	"access_token":         {},
	"api_key":              {},
	"apikey":               {},
	"auth":                 {},
	"authorization":        {},
	"bearer":               {},
	"credential":           {},
	"id_token":             {},
	"key":                  {},
	"password":             {},
	"passwd":               {},
	"refresh_token":        {},
	"secret":               {},
	"sig":                  {},
	"signature":            {},
	"token":                {},
	"x-amz-credential":     {},
	"x-amz-security-token": {},
	"x-amz-signature":      {},
	"x-goog-credential":    {},
	"x-goog-signature":     {},
	"x-ms-signature":       {},
}

func tracingEnabled(cfg TraceConfig) bool {
	return cfg.Mode != TraceOff
}

func newExecutionTraceRecorder(ctx context.Context, sessionID, executionID string, cfg TraceConfig, capture bool) (trace.Recorder, *trace.Buffer) {
	if !tracingEnabled(cfg) {
		return trace.NopRecorder{}, nil
	}

	recorders := make([]trace.Recorder, 0, 2)
	var buffer *trace.Buffer
	if capture {
		buffer = trace.NewBuffer()
		recorders = append(recorders, buffer)
	}
	if cfg.OnEvent != nil {
		recorders = append(recorders, traceCallbackRecorder{
			ctx: ctx,
			fn:  cfg.OnEvent,
		})
	}
	if len(recorders) == 0 {
		return trace.NopRecorder{}, nil
	}

	var next trace.Recorder
	if len(recorders) == 1 {
		next = recorders[0]
	} else {
		next = trace.NewFanout(recorders...)
	}

	return observabilityTraceRecorder{
		next:        next,
		mode:        cfg.Mode,
		sessionID:   sessionID,
		executionID: executionID,
	}, buffer
}

type traceCallbackRecorder struct {
	ctx context.Context //nolint:containedctx // intentional: recorder passes ctx to callback
	fn  func(context.Context, trace.Event)
}

func (r traceCallbackRecorder) Record(event *trace.Event) {
	if event == nil || r.fn == nil {
		return
	}
	cloned := cloneTraceEvent(event)
	safeTraceCallback(r.ctx, r.fn, &cloned)
}

func (traceCallbackRecorder) Snapshot() []trace.Event { return nil }

type observabilityTraceRecorder struct {
	next        trace.Recorder
	mode        TraceMode
	sessionID   string
	executionID string
}

func (r observabilityTraceRecorder) Record(event *trace.Event) {
	if event == nil || r.next == nil {
		return
	}

	prepared := cloneTraceEvent(event)
	if prepared.Schema == "" {
		prepared.Schema = trace.SchemaVersion
	}
	if prepared.SessionID == "" {
		prepared.SessionID = r.sessionID
	}
	if prepared.ExecutionID == "" {
		prepared.ExecutionID = r.executionID
	}
	if r.mode == TraceRedacted {
		prepared.Redacted = redactTraceEvent(&prepared) || prepared.Redacted
	}

	r.next.Record(&prepared)
}

func (r observabilityTraceRecorder) Snapshot() []trace.Event {
	if r.next == nil {
		return nil
	}
	return r.next.Snapshot()
}

func safeTraceCallback(ctx context.Context, fn func(context.Context, trace.Event), event *trace.Event) {
	if event == nil {
		return
	}
	defer func() { _ = recover() }()
	fn(ctx, *event)
}

func safeLogCallback(ctx context.Context, fn LogCallback, event *LogEvent) {
	if fn == nil {
		return
	}
	defer func() { _ = recover() }()
	if event != nil {
		fn(ctx, *event)
	}
}

func logExecutionEvent(ctx context.Context, fn LogCallback, event *LogEvent) {
	if fn == nil {
		return
	}
	safeLogCallback(ctx, fn, event)
}

func logExecutionOutputs(ctx context.Context, fn LogCallback, base *LogEvent, result *ExecutionResult) {
	if fn == nil || base == nil || result == nil {
		return
	}

	if result.Stdout != "" {
		event := *base
		event.Kind = LogStdout
		event.Output = result.Stdout
		event.Truncated = result.StdoutTruncated
		logExecutionEvent(ctx, fn, &event)
	}

	if result.Stderr != "" {
		event := *base
		event.Kind = LogStderr
		event.Output = result.Stderr
		event.Truncated = result.StderrTruncated
		logExecutionEvent(ctx, fn, &event)
	}
}

func logExecutionCompletion(ctx context.Context, fn LogCallback, base *LogEvent, result *ExecutionResult, err error, unexpected bool) {
	if fn == nil || base == nil || result == nil {
		return
	}

	event := *base
	event.ExitCode = result.ExitCode
	event.Duration = result.Duration
	event.ShellExited = result.ShellExited
	if unexpected && err != nil {
		event.Kind = LogExecError
		event.Error = err.Error()
		logExecutionEvent(ctx, fn, &event)
		return
	}

	event.Kind = LogExecFinish
	logExecutionEvent(ctx, fn, &event)
}

type layoutMutationRecorder struct {
	layout *sandboxLayoutState
}

func (r layoutMutationRecorder) Record(event *trace.Event) {
	if event == nil || event.Kind != trace.EventFileMutation || event.File == nil || r.layout == nil {
		return
	}
	r.layout.observeFileMutation(event.File)
}

func (layoutMutationRecorder) Snapshot() []trace.Event { return nil }

func redactTraceEvent(event *trace.Event) bool {
	if event == nil || event.Command == nil || len(event.Command.Argv) == 0 {
		return false
	}

	argv, changed := redactArgv(event.Command.Argv)
	if changed {
		event.Command.Argv = argv
	}
	return changed
}

func redactArgv(argv []string) ([]string, bool) {
	if len(argv) == 0 {
		return nil, false
	}

	out := cloneStrings(argv)
	changed := false
	for i := range out {
		arg := out[i]

		if i > 0 {
			if kind := pendingFlagValueKind(argv[i-1]); kind != flagValueNone {
				if redacted, ok := redactFlagValue(kind, arg); ok {
					if redacted != arg {
						out[i] = redacted
						changed = true
					}
					continue
				}
			}
		}

		if redacted, ok := redactInlineFlagValue(arg); ok {
			if redacted != arg {
				out[i] = redacted
				changed = true
			}
			continue
		}

		if redacted, ok := redactAttachedShortFlagValue(arg); ok {
			if redacted != arg {
				out[i] = redacted
				changed = true
			}
			continue
		}

		if redacted, ok := redactStandaloneArg(arg); ok {
			if redacted != arg {
				out[i] = redacted
				changed = true
			}
		}
	}

	return out, changed
}

// flagValueKind classifies how the value of a flag is redacted.
type flagValueKind int

const (
	flagValueNone flagValueKind = iota
	// flagValueSecret values are always redacted (-u user:pass, --token).
	flagValueSecret
	// flagValueHeader values are "Name: value" headers (-H).
	flagValueHeader
	// flagValueData values are request bodies whose key=value pairs or JSON
	// members are redacted when the key looks secret (-d, --form).
	flagValueData
)

var longFlagValueKinds = map[string]flagValueKind{
	"--header":         flagValueHeader,
	"--proxy-header":   flagValueHeader,
	"--user":           flagValueSecret,
	"--proxy-user":     flagValueSecret,
	"--oauth2-bearer":  flagValueSecret,
	"--password":       flagValueSecret,
	"--passwd":         flagValueSecret,
	"--token":          flagValueSecret,
	"--access-token":   flagValueSecret,
	"--data":           flagValueData,
	"--data-ascii":     flagValueData,
	"--data-binary":    flagValueData,
	"--data-raw":       flagValueData,
	"--data-urlencode": flagValueData,
	"--form":           flagValueData,
	"--form-string":    flagValueData,
	"--json":           flagValueData,
}

// shortFlagValueKind maps short flags whose value may carry secrets. These
// follow curl's spelling; short flags may be grouped (-su) or take an
// attached value (-uuser:pass).
func shortFlagValueKind(flag byte) flagValueKind {
	switch flag {
	case 'u', 'U':
		return flagValueSecret
	case 'H':
		return flagValueHeader
	case 'd', 'F':
		return flagValueData
	default:
		return flagValueNone
	}
}

// pendingFlagValueKind reports how the argument following flag should be
// redacted, recognizing long flags and short-flag groups ending in a
// value-taking flag (-H, -su).
func pendingFlagValueKind(flag string) flagValueKind {
	if kind, ok := longFlagValueKinds[flag]; ok {
		return kind
	}
	if !isShortFlagGroup(flag) {
		return flagValueNone
	}
	return shortFlagValueKind(flag[len(flag)-1])
}

func isShortFlagGroup(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
		return false
	}
	for i := 1; i < len(arg); i++ {
		if !isASCIILetter(arg[i]) {
			return false
		}
	}
	return true
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func redactFlagValue(kind flagValueKind, value string) (string, bool) {
	switch kind {
	case flagValueSecret:
		return redactedValue, true
	case flagValueHeader:
		if redacted, ok := redactHeaderValue(value); ok {
			return redacted, true
		}
	case flagValueData:
		if redacted, ok := redactDataValue(value); ok {
			return redacted, true
		}
	}
	return redactStandaloneArg(value)
}

func redactInlineFlagValue(arg string) (string, bool) {
	prefix, value, ok := strings.Cut(arg, "=")
	if !ok || !strings.HasPrefix(prefix, "--") {
		return "", false
	}

	if redacted, changed := redactFlagValue(longFlagValueKinds[prefix], value); changed {
		return prefix + "=" + redacted, true
	}
	return "", false
}

// redactAttachedShortFlagValue handles values attached to a short flag,
// optionally after other grouped flags: -uuser:pass, -sHAuthorization:...,
// -dpassword=x. The first value-taking flag in the group consumes the rest.
func redactAttachedShortFlagValue(arg string) (string, bool) {
	if len(arg) < 3 || arg[0] != '-' || arg[1] == '-' {
		return "", false
	}
	for i := 1; i < len(arg)-1; i++ {
		if !isASCIILetter(arg[i]) {
			return "", false
		}
		kind := shortFlagValueKind(arg[i])
		if kind == flagValueNone {
			continue
		}
		prefix, value := arg[:i+1], arg[i+1:]
		if kind == flagValueSecret {
			// "user:pass" is the secret-bearing form; requiring the colon
			// keeps grouped flags such as "sort -ur" readable.
			if !strings.Contains(value, ":") {
				return "", false
			}
			return prefix + redactedValue, true
		}
		if redacted, ok := redactFlagValue(kind, value); ok {
			return prefix + redacted, true
		}
		return "", false
	}
	return "", false
}

func redactStandaloneArg(arg string) (string, bool) {
	if redacted, changed := redactHeaderValue(arg); changed {
		return redacted, true
	}
	if redacted, changed := redactURLValue(arg); changed {
		return redacted, true
	}
	if redacted, changed := redactBearerValue(arg); changed {
		return redacted, true
	}
	return "", false
}

func redactHeaderValue(value string) (string, bool) {
	name, headerValue, ok := strings.Cut(value, ":")
	if !ok {
		return "", false
	}

	normalizedName := strings.TrimSpace(name)
	if !isHeaderName(normalizedName) {
		return "", false
	}
	if !isSensitiveName(normalizedName) && !looksSensitiveHeaderValue(headerValue) {
		return "", false
	}
	return normalizedName + ": " + redactedValue, true
}

// isHeaderName reports whether name is an HTTP header field name token, so
// that URLs ("https://...") and other colon-bearing args are not mistaken
// for headers.
func isHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if isASCIILetter(c) || (c >= '0' && c <= '9') || strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0 {
			continue
		}
		return false
	}
	return true
}

// sensitiveNameFragments mark a header, query, or form key as secret when
// they appear anywhere in it (PRIVATE-TOKEN, client_secret, X-Auth-User).
var sensitiveNameFragments = []string{
	"token",
	"secret",
	"passw",
	"auth",
	"cookie",
	"credential",
	"signature",
	"apikey",
	"session",
}

// sensitiveNameComponents mark a key as secret when they appear as a whole
// "-", "_", or "." separated component (X-Api-Key, api_key, sig).
var sensitiveNameComponents = map[string]struct{}{
	"key": {},
	"sig": {},
	"pwd": {},
	"jwt": {},
}

func isSensitiveName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return false
	}
	if _, ok := sensitiveQueryKeys[normalized]; ok {
		return true
	}
	for _, fragment := range sensitiveNameFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	for component := range strings.FieldsFuncSeq(normalized, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	}) {
		if _, ok := sensitiveNameComponents[component]; ok {
			return true
		}
	}
	return false
}

func looksSensitiveHeaderValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(normalized, "bearer ") ||
		strings.HasPrefix(normalized, "basic ") ||
		strings.HasPrefix(normalized, "digest ") ||
		strings.HasPrefix(normalized, "token ")
}

func redactBearerValue(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(strings.ToLower(trimmed), "bearer "):
		return "Bearer " + redactedValue, true
	case strings.HasPrefix(strings.ToLower(trimmed), "basic "):
		return "Basic " + redactedValue, true
	default:
		return "", false
	}
}

func redactURLValue(value string) (string, bool) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}
	if parsed.Scheme == "" && parsed.Host == "" {
		return "", false
	}

	changed := false
	hadUserinfo := parsed.User != nil && parsed.Host != ""
	if hadUserinfo {
		parsed.User = nil
		changed = true
	}
	if parsed.RawQuery != "" {
		if redactedQuery, queryChanged := redactRawQuery(parsed.RawQuery); queryChanged {
			parsed.RawQuery = redactedQuery
			changed = true
		}
	}
	if !changed {
		return "", false
	}

	out := parsed.String()
	if hadUserinfo {
		// url.Userinfo would percent-encode the brackets, so splice the
		// marker in after the authority's "//".
		out = strings.Replace(out, "//", "//"+redactedValue+"@", 1)
	}
	return out, true
}

func redactRawQuery(raw string) (string, bool) {
	parts := strings.Split(raw, "&")
	changed := false
	for i, part := range parts {
		if part == "" {
			continue
		}

		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}

		decodedKey, err := url.QueryUnescape(key)
		if err != nil {
			decodedKey = key
		}
		if !isSensitiveName(decodedKey) {
			continue
		}

		if value != redactedValue {
			parts[i] = key + "=" + redactedValue
			changed = true
		}
	}

	return strings.Join(parts, "&"), changed
}

// jsonStringMember matches a "key": "value" member of a JSON object body.
var jsonStringMember = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"(\s*:\s*)"(?:[^"\\]|\\.)*"`)

// redactDataValue redacts secret-looking fields of a request body passed to
// -d/--data/--form: key=value pairs separated by "&" and string members of a
// JSON object.
func redactDataValue(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		changed := false
		out := jsonStringMember.ReplaceAllStringFunc(value, func(member string) string {
			match := jsonStringMember.FindStringSubmatch(member)
			if !isSensitiveName(match[1]) {
				return member
			}
			changed = true
			return `"` + match[1] + `"` + match[2] + `"` + redactedValue + `"`
		})
		return out, changed
	}
	if !strings.Contains(value, "=") {
		return "", false
	}
	return redactRawQuery(value)
}
