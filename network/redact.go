package network

import (
	"net/url"
	"regexp"
	"strings"
)

// RedactedValue replaces secrets in redacted URLs, headers and arguments.
const RedactedValue = "[REDACTED]"

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

// IsSensitiveName reports whether a header, query, or form key looks like it
// carries a secret.
func IsSensitiveName(name string) bool {
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

// RedactURL strips userinfo and secret-looking query values from an absolute
// URL. It reports false when value is not a URL or nothing was redacted.
func RedactURL(value string) (string, bool) {
	parsed, err := url.Parse(value)
	if err != nil {
		return redactUnparsedURL(value)
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
		if redactedQuery, queryChanged := RedactQuery(parsed.RawQuery); queryChanged {
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
		out = strings.Replace(out, "//", "//"+RedactedValue+"@", 1)
	}
	return out, true
}

// RedactQuery redacts the values of secret-looking keys in a
// "k=v&k=v" query or form string.
func RedactQuery(raw string) (string, bool) {
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
		if !IsSensitiveName(decodedKey) {
			continue
		}

		if value != RedactedValue {
			parts[i] = key + "=" + RedactedValue
			changed = true
		}
	}

	return strings.Join(parts, "&"), changed
}

// unparsedUserinfo matches the userinfo of a URL authority up to the last "@"
// before the path, query, or fragment.
var unparsedUserinfo = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*://)[^/?#]*@`)

// redactUnparsedURL handles URL-like values that url.Parse rejects (such as an
// unescaped "%" in a password) so secrets are not echoed verbatim.
func redactUnparsedURL(value string) (string, bool) {
	if !strings.Contains(value, "://") {
		return "", false
	}
	out := value
	changed := false
	if unparsedUserinfo.MatchString(out) {
		out = unparsedUserinfo.ReplaceAllString(out, "${1}"+RedactedValue+"@")
		changed = true
	}
	if base, query, ok := strings.Cut(out, "?"); ok {
		query, fragment, hasFragment := strings.Cut(query, "#")
		if redacted, queryChanged := RedactQuery(query); queryChanged {
			out = base + "?" + redacted
			if hasFragment {
				out += "#" + fragment
			}
			changed = true
		}
	}
	return out, changed
}
