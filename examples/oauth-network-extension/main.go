package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/network"
	"github.com/ewhauser/gbash/trace"
)

const (
	demoRequestURL       = "https://crm.example.test/v1/profile"
	requestID            = "sandbox-demo-42"
	overrideAttemptID    = "sandbox-spoof-43"
	sandboxForgedAuth    = "Bearer sandbox-forged-token"
	tokenRef             = "vault://crm-api/oauth"
	baselineScenarioName = "host injects oauth"
	overrideScenarioName = "sandbox authorization is overridden"
)

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, stdout io.Writer) error {
	report, err := runDemo(ctx)
	if err != nil {
		return err
	}
	return renderReport(stdout, report)
}

func runDemo(ctx context.Context) (*demoReport, error) {
	vault := newDemoVault()
	server := newDemoAPIServer(vault)
	defer server.Close()

	client, err := newOAuthInjectingClient(server.URL(), vault)
	if err != nil {
		return nil, fmt.Errorf("create oauth client: %w", err)
	}

	rt, err := gbash.New(
		gbash.WithNetworkClient(client),
		gbash.WithTracing(gbash.TraceConfig{Mode: gbash.TraceRaw}),
	)
	if err != nil {
		return nil, fmt.Errorf("create runtime: %w", err)
	}

	baselineScript := fmt.Sprintf("curl -fsS -H 'X-Request-ID: %s' %s\n", requestID, demoRequestURL)
	baseline, err := runScenario(ctx, rt, client, server, scenarioSpec{
		Name:      baselineScenarioName,
		RequestID: requestID,
		Script:    baselineScript,
	})
	if err != nil {
		return nil, err
	}

	overrideScript := fmt.Sprintf(
		"curl -fsS -H 'Authorization: %s' -H 'X-Request-ID: %s' %s\n",
		sandboxForgedAuth,
		overrideAttemptID,
		demoRequestURL,
	)
	override, err := runScenario(ctx, rt, client, server, scenarioSpec{
		Name:      overrideScenarioName,
		RequestID: overrideAttemptID,
		Script:    overrideScript,
	})
	if err != nil {
		return nil, err
	}

	if !override.Audit.SandboxAuthorizationIn {
		return nil, errors.New("demo failed: override scenario did not observe the sandbox authorization header")
	}
	if !override.Audit.AuthorizationOverrideApplied {
		return nil, errors.New("demo failed: extension did not override the sandbox authorization header")
	}

	return &demoReport{
		Scenarios: []scenarioReport{baseline, override},
	}, nil
}

type scenarioSpec struct {
	Name      string
	RequestID string
	Script    string
}

func runScenario(ctx context.Context, rt *gbash.Runtime, client *oauthInjectingClient, server *demoAPIServer, spec scenarioSpec) (scenarioReport, error) {
	result, err := rt.Run(ctx, &gbash.ExecutionRequest{
		Name:   "oauth-network-extension",
		Script: spec.Script,
	})
	if err != nil {
		return scenarioReport{}, fmt.Errorf("%s: run script: %w", spec.Name, err)
	}
	if result.ExitCode != 0 {
		return scenarioReport{}, fmt.Errorf("%s: curl exited with %d: %s", spec.Name, result.ExitCode, strings.TrimSpace(result.Stderr))
	}

	response, err := decodeAPIResponse(result.Stdout)
	if err != nil {
		return scenarioReport{}, fmt.Errorf("%s: %w", spec.Name, err)
	}

	traceArgv, err := findCurlArgv(result.Events)
	if err != nil {
		return scenarioReport{}, fmt.Errorf("%s: %w", spec.Name, err)
	}

	audit := client.LastAudit()
	serverRequest := server.LastRequest()
	token := client.vault.mustSecret(tokenRef).Token

	report := scenarioReport{
		Name:                  spec.Name,
		Script:                spec.Script,
		CurlStdout:            result.Stdout,
		TraceArgv:             traceArgv,
		Response:              response,
		Audit:                 audit,
		ServerRequest:         serverRequest,
		SecretVisibleInStdout: strings.Contains(result.Stdout, token),
		SecretVisibleInStderr: strings.Contains(result.Stderr, token),
		SecretVisibleInTrace:  strings.Contains(strings.Join(traceArgv, " "), token),
	}

	if report.Response.RequestID != spec.RequestID {
		return scenarioReport{}, fmt.Errorf("%s: response request id = %q, want %q", spec.Name, report.Response.RequestID, spec.RequestID)
	}
	if report.ServerRequest.RequestID != spec.RequestID {
		return scenarioReport{}, fmt.Errorf("%s: server request id = %q, want %q", spec.Name, report.ServerRequest.RequestID, spec.RequestID)
	}
	if !report.Audit.AuthorizationInjected {
		return scenarioReport{}, fmt.Errorf("%s: oauth header was not injected", spec.Name)
	}
	if !report.ServerRequest.AuthorizationValid {
		return scenarioReport{}, fmt.Errorf("%s: server did not receive the injected oauth token", spec.Name)
	}
	if report.SecretVisibleInStdout || report.SecretVisibleInStderr || report.SecretVisibleInTrace {
		return scenarioReport{}, fmt.Errorf("%s: oauth token leaked back into sandbox-visible output", spec.Name)
	}

	return report, nil
}

type demoVault struct {
	secrets map[string]oauthSecret
}

type oauthSecret struct {
	Token   string
	Subject string
	Scope   string
}

func newDemoVault() *demoVault {
	return &demoVault{
		secrets: map[string]oauthSecret{
			tokenRef: {
				Token:   "demo-oauth-access-token",
				Subject: "demo-service-account",
				Scope:   "crm.read",
			},
		},
	}
}

func (v *demoVault) mustSecret(ref string) oauthSecret {
	secret, ok := v.secrets[ref]
	if !ok {
		panic("missing demo vault secret: " + ref)
	}
	return secret
}

type demoAPIServer struct {
	server      *httptest.Server
	vault       *demoVault
	mu          sync.Mutex
	lastRequest serverRequestAudit
}

type serverRequestAudit struct {
	Method               string
	Path                 string
	RequestID            string
	AuthorizationPresent bool
	AuthorizationValid   bool
}

func newDemoAPIServer(vault *demoVault) *demoAPIServer {
	demo := &demoAPIServer{vault: vault}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/profile", demo.handleProfile)
	demo.server = httptest.NewServer(mux)
	return demo
}

func (s *demoAPIServer) URL() string {
	return s.server.URL
}

func (s *demoAPIServer) Close() {
	s.server.Close()
}

func (s *demoAPIServer) handleProfile(w http.ResponseWriter, r *http.Request) {
	expectedAuth := "Bearer " + s.vault.mustSecret(tokenRef).Token
	gotAuth := r.Header.Get("Authorization")

	s.mu.Lock()
	s.lastRequest = serverRequestAudit{
		Method:               r.Method,
		Path:                 r.URL.Path,
		RequestID:            r.Header.Get("X-Request-ID"),
		AuthorizationPresent: gotAuth != "",
		AuthorizationValid:   gotAuth == expectedAuth,
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if gotAuth != expectedAuth {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(apiResponse{
			OK:              false,
			Error:           "missing or invalid bearer token",
			RequestID:       r.Header.Get("X-Request-ID"),
			AuthenticatedAs: "",
		})
		return
	}

	secret := s.vault.mustSecret(tokenRef)
	_ = json.NewEncoder(w).Encode(apiResponse{
		OK:                  true,
		Service:             "crm",
		AuthenticatedAs:     secret.Subject,
		AuthorizationSource: "host-extension-vault",
		RequestID:           r.Header.Get("X-Request-ID"),
		Scope:               secret.Scope,
	})
}

func (s *demoAPIServer) LastRequest() serverRequestAudit {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRequest
}

type apiResponse struct {
	OK                  bool   `json:"ok"`
	Service             string `json:"service,omitempty"`
	AuthenticatedAs     string `json:"authenticated_as,omitempty"`
	AuthorizationSource string `json:"authorization_source,omitempty"`
	RequestID           string `json:"request_id,omitempty"`
	Scope               string `json:"scope,omitempty"`
	Error               string `json:"error,omitempty"`
}

func decodeAPIResponse(stdout string) (apiResponse, error) {
	var response apiResponse
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		return apiResponse{}, fmt.Errorf("decode curl stdout as json: %w", err)
	}
	return response, nil
}

type oauthInjectingClient struct {
	httpClient *http.Client
	serverURL  *url.URL
	vault      *demoVault
	mu         sync.Mutex
	lastAudit  requestAudit
}

type requestAudit struct {
	LogicalURL                   string
	ForwardedURL                 string
	AuthorizationInjected        bool
	AuthorizationSource          string
	SandboxAuthorizationIn       bool
	AuthorizationOverrideApplied bool
}

func newOAuthInjectingClient(serverURL string, vault *demoVault) (*oauthInjectingClient, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse server url: %w", err)
	}
	return &oauthInjectingClient{
		httpClient: &http.Client{},
		serverURL:  parsed,
		vault:      vault,
	}, nil
}

func (c *oauthInjectingClient) Do(ctx context.Context, req *network.Request) (*network.Response, error) {
	if req == nil {
		return nil, errors.New("network request was nil")
	}

	logicalURL, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("parse logical request url: %w", err)
	}
	if logicalURL.Host != "crm.example.test" {
		return nil, &network.AccessDeniedError{
			URL:    req.URL,
			Reason: "host not registered by oauth network extension",
		}
	}

	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = http.MethodGet
	}

	forwarded := *c.serverURL
	forwarded.Path = logicalURL.Path
	forwarded.RawQuery = logicalURL.RawQuery

	requestCtx := ctx
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, req.Timeout.Round(time.Millisecond))
		defer cancel()
	}

	httpReq, err := http.NewRequestWithContext(requestCtx, method, forwarded.String(), bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("create forwarded request: %w", err)
	}
	for name, value := range req.Headers {
		httpReq.Header.Set(name, value)
	}

	secret := c.vault.mustSecret(tokenRef)
	incomingAuthorization := req.Headers["Authorization"]
	httpReq.Header.Set("Authorization", "Bearer "+secret.Token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("perform forwarded request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	c.mu.Lock()
	c.lastAudit = requestAudit{
		LogicalURL:                   req.URL,
		ForwardedURL:                 forwarded.String(),
		AuthorizationInjected:        true,
		AuthorizationSource:          tokenRef,
		SandboxAuthorizationIn:       incomingAuthorization != "",
		AuthorizationOverrideApplied: incomingAuthorization != "",
	}
	c.mu.Unlock()

	return &network.Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    flattenHeaders(resp.Header),
		Body:       body,
		URL:        req.URL,
	}, nil
}

func (c *oauthInjectingClient) LastAudit() requestAudit {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastAudit
}

func flattenHeaders(header http.Header) map[string]string {
	out := make(map[string]string, len(header))
	for name, values := range header {
		out[name] = strings.Join(values, ", ")
	}
	return out
}

type demoReport struct {
	Scenarios []scenarioReport
}

type scenarioReport struct {
	Name                  string
	Script                string
	CurlStdout            string
	TraceArgv             []string
	Response              apiResponse
	Audit                 requestAudit
	ServerRequest         serverRequestAudit
	SecretVisibleInStdout bool
	SecretVisibleInStderr bool
	SecretVisibleInTrace  bool
}

func renderReport(w io.Writer, report *demoReport) error {
	if _, err := fmt.Fprintln(w, "gbash oauth network extension demo"); err != nil {
		return err
	}
	for i := range report.Scenarios {
		scenario := report.Scenarios[i]
		traceJSON, _ := json.Marshal(scenario.TraceArgv)

		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Scenario %d: %s\n", i+1, scenario.Name); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Sandbox script:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %s", strings.TrimSpace(scenario.Script)); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Curl stdout:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %s\n", strings.TrimSpace(scenario.CurlStdout)); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Trace argv:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %s\n", traceJSON); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Host-side audit:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  logical_url=%s\n", scenario.Audit.LogicalURL); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  forwarded_url=%s\n", scenario.Audit.ForwardedURL); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  authorization_source=%s\n", scenario.Audit.AuthorizationSource); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  authorization_injected=%t\n", scenario.Audit.AuthorizationInjected); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  sandbox_sent_authorization=%t\n", scenario.Audit.SandboxAuthorizationIn); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  authorization_override_applied=%t\n", scenario.Audit.AuthorizationOverrideApplied); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Server-side verification:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  request_id=%s\n", scenario.ServerRequest.RequestID); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  authorization_present=%t\n", scenario.ServerRequest.AuthorizationPresent); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  authorization_valid=%t\n", scenario.ServerRequest.AuthorizationValid); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Leak checks:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  secret_visible_in_stdout=%t\n", scenario.SecretVisibleInStdout); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  secret_visible_in_stderr=%t\n", scenario.SecretVisibleInStderr); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  secret_visible_in_trace=%t\n", scenario.SecretVisibleInTrace); err != nil {
			return err
		}
	}
	return nil
}

func findCurlArgv(events []trace.Event) ([]string, error) {
	for i := range events {
		event := events[i]
		if event.Kind != trace.EventCallExpanded || event.Command == nil {
			continue
		}
		if event.Command.Name != "curl" {
			continue
		}
		return append([]string(nil), event.Command.Argv...), nil
	}
	return nil, errors.New("trace did not include curl argv")
}
