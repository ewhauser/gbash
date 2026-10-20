package runtime

import (
	"reflect"
	"testing"
)

func TestRedactArgvCoversCommonSecretForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "bash -c script",
			argv: []string{"bash", "-c", "curl -u admin:HUNTER2 https://example.test/"},
			want: []string{"bash", "-c", "curl -u '[REDACTED]' https://example.test/"},
		},
		{
			name: "sh -ec quoted script",
			argv: []string{"sh", "-ec", `curl -H "X-Api-Key: HUNTER2" https://example.test/`},
			want: []string{"sh", "-ec", `curl -H 'X-Api-Key: [REDACTED]' https://example.test/`},
		},
		{
			name: "eval",
			argv: []string{"eval", "curl --token HUNTER2 x"},
			want: []string{"eval", "curl --token '[REDACTED]' x"},
		},
		{
			name: "cookie and cert flags",
			argv: []string{"curl", "-b", "session=abc", "--cookie=s=1", "-E", "c.pem:pw", "--pass", "pw", "-bsid=x"},
			want: []string{"curl", "-b", "[REDACTED]", "--cookie=[REDACTED]", "-E", "[REDACTED]", "--pass", "[REDACTED]", "-b[REDACTED]"},
		},
		{
			name: "unparseable URL userinfo",
			argv: []string{"curl", "https://admin:p%zz@example.test/x?token=abc"},
			want: []string{"curl", "https://[REDACTED]@example.test/x?token=[REDACTED]"},
		},
		{
			name: "userinfo without query",
			argv: []string{"curl", "https://user:pass@example.test/"},
			want: []string{"curl", "https://[REDACTED]@example.test/"},
		},
		{
			name: "userinfo with query",
			argv: []string{"curl", "https://user:pass@example.test/p?token=t&ok=1"},
			want: []string{"curl", "https://[REDACTED]@example.test/p?token=[REDACTED]&ok=1"},
		},
		{
			name: "userinfo in inline flag value",
			argv: []string{"curl", "--proxy=http://user:pass@proxy.test:8080"},
			want: []string{"curl", "--proxy=http://[REDACTED]@proxy.test:8080"},
		},
		{
			name: "attached -u",
			argv: []string{"curl", "-uuser:pass", "https://example.test/"},
			want: []string{"curl", "-u[REDACTED]", "https://example.test/"},
		},
		{
			name: "grouped -su",
			argv: []string{"curl", "-su", "user:pass", "https://example.test/"},
			want: []string{"curl", "-su", "[REDACTED]", "https://example.test/"},
		},
		{
			name: "grouped attached -suuser:pass",
			argv: []string{"curl", "-suuser:pass"},
			want: []string{"curl", "-su[REDACTED]"},
		},
		{
			name: "attached -H",
			argv: []string{"curl", "-HAuthorization: Bearer x"},
			want: []string{"curl", "-HAuthorization: [REDACTED]"},
		},
		{
			name: "grouped -sH",
			argv: []string{"curl", "-sH", "PRIVATE-TOKEN: glpat-x"},
			want: []string{"curl", "-sH", "PRIVATE-TOKEN: [REDACTED]"},
		},
		{
			name: "broader header names",
			argv: []string{"curl", "-H", "X-Goog-Api-Key: k", "-H", "X-Api-Key: k", "--header", "Ocp-Apim-Subscription-Key: k", "-H", "X-Session-Id: s", "-H", "Accept: text/plain"},
			want: []string{"curl", "-H", "X-Goog-Api-Key: [REDACTED]", "-H", "X-Api-Key: [REDACTED]", "--header", "Ocp-Apim-Subscription-Key: [REDACTED]", "-H", "X-Session-Id: [REDACTED]", "-H", "Accept: text/plain"},
		},
		{
			name: "form data",
			argv: []string{"curl", "--data", "user=bob&password=hunter2", "-d", "client_secret=s", "--data-urlencode=api_key=k", "-dpassword=p", "-F", "name=x"},
			want: []string{"curl", "--data", "user=bob&password=[REDACTED]", "-d", "client_secret=[REDACTED]", "--data-urlencode=api_key=[REDACTED]", "-d" + "password=[REDACTED]", "-F", "name=x"},
		},
		{
			name: "json data",
			argv: []string{"curl", "--json", `{"user":"bob","password":"hunter2","nested":{"accessToken":"t"}}`},
			want: []string{"curl", "--json", `{"user":"bob","password":"[REDACTED]","nested":{"accessToken":"[REDACTED]"}}`},
		},
		{
			name: "proxy user",
			argv: []string{"curl", "--proxy-user", "u:p", "-U", "u:p"},
			want: []string{"curl", "--proxy-user", "[REDACTED]", "-U", "[REDACTED]"},
		},
		{
			name: "broader query keys",
			argv: []string{"curl", "https://example.test/?private_token=t&X-Goog-Api-Key=k&page=2"},
			want: []string{"curl", "https://example.test/?private_token=[REDACTED]&X-Goog-Api-Key=[REDACTED]&page=2"},
		},
		{
			name: "ordinary args untouched",
			argv: []string{"sort", "-ur", "-o", "out.txt", "cut", "-d:", "-f1", "ls", "-lH", "dir", "https://example.test/a?page=1"},
			want: []string{"sort", "-ur", "-o", "out.txt", "cut", "-d:", "-f1", "ls", "-lH", "dir", "https://example.test/a?page=1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, changed := redactArgv(tt.argv)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("redactArgv(%q) = %q, want %q", tt.argv, got, tt.want)
			}
			if wantChanged := !reflect.DeepEqual(tt.argv, tt.want); changed != wantChanged {
				t.Fatalf("redactArgv(%q) changed = %t, want %t", tt.argv, changed, wantChanged)
			}
		})
	}
}

func TestTraceRedactedRedactsCurlCredentialForms(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{
		Tracing: TraceConfig{Mode: TraceRedacted},
	})

	result := mustExecSession(t, session, "echo https://user:pass@example.test/ -uuser:pass -su user:pass --data password=hunter2 -H 'PRIVATE-TOKEN: glpat-secret'\n")
	event := findCommandEvent(result.Events)
	if event == nil || event.Command == nil {
		t.Fatalf("missing redacted command event: %#v", result.Events)
	}
	if !event.Redacted {
		t.Fatalf("event.Redacted = %t, want true", event.Redacted)
	}
	if got, want := event.Command.Argv, []string{
		"echo",
		"https://[REDACTED]@example.test/",
		"-u[REDACTED]",
		"-su",
		"[REDACTED]",
		"--data",
		"password=[REDACTED]",
		"-H",
		"PRIVATE-TOKEN: [REDACTED]",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("redacted argv = %#v, want %#v", got, want)
	}
}
