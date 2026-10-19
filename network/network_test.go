package network

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
)

type resolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (fn resolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return fn(ctx, host)
}

type sequenceResolver struct {
	mu        sync.Mutex
	responses [][]net.IPAddr
}

func (r *sequenceResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.responses) == 0 {
		return nil, &net.DNSError{Err: "no such host", IsNotFound: true}
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	return response, nil
}

type staticHTTPDoer struct{}

func (staticHTTPDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

func TestNewRejectsInvalidAllowList(t *testing.T) {
	t.Parallel()
	_, err := New(&Config{
		AllowedURLPrefixes: []string{"example.com"},
	})
	if err == nil {
		t.Fatal("New() error = nil, want invalid config error")
	}
}

func TestClientAllowsMatchingOriginAndPathPrefix(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{server.URL + "/v1/"},
		DenyPrivateRanges:  false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := client.Do(context.Background(), &Request{
		URL: server.URL + "/v1/users",
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got, want := string(resp.Body), "ok"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}
}

func TestClientBlocksPathOutsideAllowListPrefix(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com/v1/"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{
		URL: "https://api.example.com/v2/users",
	})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied error", err)
	}
}

func TestClientTreatsAllowListPathsAsSegmentBoundaries(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com/private"},
	}, WithDoer(staticHTTPDoer{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, target := range []string{
		"https://api.example.com/private",
		"https://api.example.com/private/token",
	} {
		_, err := client.Do(context.Background(), &Request{URL: target})
		if err != nil {
			t.Fatalf("Do(%q) error = %v, want allowed request", target, err)
		}
	}

	_, err = client.Do(context.Background(), &Request{
		URL: "https://api.example.com/private-token",
	})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied sibling-path request", err)
	}
}

func TestClientBlocksDisallowedMethod(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{
		Method: "POST",
		URL:    "https://api.example.com/items",
	})
	var methodErr *MethodNotAllowedError
	if !errors.As(err, &methodErr) {
		t.Fatalf("Do() error = %v, want method denied error", err)
	}
}

func TestClientRevalidatesRedirectTargets(t *testing.T) {
	t.Parallel()
	deniedURL := "https://other.example.com/blocked"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, deniedURL, http.StatusFound)
	}))
	defer server.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{server.URL},
		DenyPrivateRanges:  false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{
		URL:             server.URL,
		FollowRedirects: true,
	})
	var redirectErr *RedirectNotAllowedError
	if !errors.As(err, &redirectErr) {
		t.Fatalf("Do() error = %v, want redirect denied error", err)
	}
}

func TestClientRevalidatesRedirectTargetsAcrossPathBoundary(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/private-token", http.StatusFound)
	}))
	defer server.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{server.URL + "/private"},
		DenyPrivateRanges:  false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{
		URL:             server.URL + "/private",
		FollowRedirects: true,
	})
	var redirectErr *RedirectNotAllowedError
	if !errors.As(err, &redirectErr) {
		t.Fatalf("Do() error = %v, want redirect denied error", err)
	}
}

func TestClientEnforcesResponseSizeLimit(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 32)))
	}))
	defer server.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{server.URL},
		MaxResponseBytes:   8,
		DenyPrivateRanges:  false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{URL: server.URL})
	var sizeErr *ResponseTooLargeError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("Do() error = %v, want response-too-large error", err)
	}
}

func TestClientBlocksPrivateRangesLexically(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"http://127.0.0.1"},
		DenyPrivateRanges:  true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{URL: "http://127.0.0.1/"})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied error", err)
	}
}

func TestClientBlocksPrivateRangesAfterDNSResolution(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com"},
		DenyPrivateRanges:  true,
	}, WithResolver(resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{URL: "https://api.example.com/path"})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied error", err)
	}
}

func TestClientBlocksPrivateRangesDuringDial(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"http://api.example.test:80"},
		DenyPrivateRanges:  true,
	}, WithResolver(&sequenceResolver{
		responses: [][]net.IPAddr{
			{{IP: net.ParseIP("93.184.216.34")}},
			{{IP: net.ParseIP("127.0.0.1")}},
		},
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{URL: "http://api.example.test:80/path"})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied error", err)
	}
}

func TestClientIgnoresHostProxyEnvironment(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("backend"))
	}))
	defer backend.Close()

	var mu sync.Mutex
	proxyHit := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proxyHit = true
		mu.Unlock()
		http.Error(w, "proxy", http.StatusBadGateway)
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")

	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", backend.URL, err)
	}
	_, port, err := net.SplitHostPort(backendURL.Host)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) error = %v", backendURL.Host, err)
	}
	allowedHost := net.JoinHostPort("allowed.example.test", port)

	client, err := New(&Config{
		AllowedURLPrefixes: []string{"http://" + allowedHost},
		DenyPrivateRanges:  false,
	}, WithResolver(resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := client.Do(context.Background(), &Request{URL: "http://" + allowedHost + "/via-default-transport"})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got, want := string(resp.Body), "backend"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}

	mu.Lock()
	hit := proxyHit
	mu.Unlock()
	if hit {
		t.Fatal("proxy server was used, want direct sandbox-controlled dial")
	}
}

type recordingHTTPDoer struct {
	mu        sync.Mutex
	requests  []string
	locations []string
}

func (d *recordingHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.requests = append(d.requests, req.URL.RequestURI())
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}
	if len(d.locations) > 0 {
		resp.StatusCode = http.StatusFound
		resp.Status = "302 Found"
		resp.Header.Set("Location", d.locations[0])
		d.locations = d.locations[1:]
	}
	return resp, nil
}

func (d *recordingHTTPDoer) sent() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.requests)
}

func TestClientBlocksDotSegmentEscapesFromAllowListPrefix(t *testing.T) {
	t.Parallel()
	doer := &recordingHTTPDoer{}
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com/v1/"},
	}, WithDoer(doer))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, target := range []string{
		"https://api.example.com/v1/../admin",
		"https://api.example.com/v1/./../admin",
		"https://api.example.com/v1/users/../../admin",
		"https://api.example.com/v1/..",
		"https://api.example.com/v1/%2e%2e/admin",
		"https://api.example.com/v1/%2E%2E/admin",
		"https://api.example.com/v1/.%2e/admin",
		"https://api.example.com/v1/%2e./admin",
		"https://api.example.com/v1/%2e%2e",
		"https://api.example.com/v1/..%2fadmin",
		"https://api.example.com/v1/..%2Fadmin",
		"https://api.example.com/v1/..%5cadmin",
		"https://api.example.com/v1/..\\admin",
		"https://api.example.com/v1/..;/admin",
		"https://api.example.com/v1/%2e%2e;/admin",
		"https://api.example.com/v1/users%2f%2e%2e%2f%2e%2e%2fadmin",
	} {
		_, err := client.Do(context.Background(), &Request{URL: target})
		if !IsDenied(err) {
			t.Errorf("Do(%q) error = %v, want denied error", target, err)
		}
	}
	if sent := doer.sent(); len(sent) != 0 {
		t.Fatalf("doer received %v, want no requests", sent)
	}
}

func TestClientNormalizesDotSegmentsBeforeSending(t *testing.T) {
	t.Parallel()
	doer := &recordingHTTPDoer{}
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com/v1/"},
	}, WithDoer(doer))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cases := map[string]string{
		"https://api.example.com/v1/./users":        "/v1/users",
		"https://api.example.com/v1/a/../users?x=1": "/v1/users?x=1",
		"https://api.example.com/v1/users/.":        "/v1/users/",
		"https://api.example.com/v1/users..json":    "/v1/users..json",
		"https://api.example.com/v1/%2e%2etxt":      "/v1/%2e%2etxt",
		"https://api.example.com/v1/a%2Fb/../users": "/v1/users",
	}
	for target, want := range cases {
		resp, err := client.Do(context.Background(), &Request{URL: target})
		if err != nil {
			t.Errorf("Do(%q) error = %v, want allowed request", target, err)
			continue
		}
		sent := doer.sent()
		if got := sent[len(sent)-1]; got != want {
			t.Errorf("Do(%q) sent %q, want %q", target, got, want)
		}
		if got, want := resp.URL, "https://api.example.com"+want; got != want {
			t.Errorf("Do(%q) Response.URL = %q, want %q", target, got, want)
		}
	}
}

func TestClientBlocksDotSegmentEscapesInRedirects(t *testing.T) {
	t.Parallel()
	for _, location := range []string{
		"/v1/%2e%2e/admin",
		"https://api.example.com/v1/%2E%2E/admin",
		"%2e%2e/%2e%2e/admin",
		"/v1/..%2fadmin",
		"/v1/..;/admin",
	} {
		doer := &recordingHTTPDoer{locations: []string{location}}
		client, err := New(&Config{
			AllowedURLPrefixes: []string{"https://api.example.com/v1/"},
		}, WithDoer(doer))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		_, err = client.Do(context.Background(), &Request{
			URL:             "https://api.example.com/v1/start",
			FollowRedirects: true,
		})
		var redirectErr *RedirectNotAllowedError
		if !errors.As(err, &redirectErr) {
			t.Errorf("redirect to %q: Do() error = %v, want redirect denied error", location, err)
		}
		if sent := doer.sent(); len(sent) != 1 {
			t.Errorf("redirect to %q: doer received %v, want only the initial request", location, sent)
		}
	}
}

func TestNewNormalizesAllowListDotSegments(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com/v1/../v2/"},
	}, WithDoer(staticHTTPDoer{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.Do(context.Background(), &Request{URL: "https://api.example.com/v2/users"}); err != nil {
		t.Fatalf("Do(v2) error = %v, want allowed", err)
	}
	if _, err := client.Do(context.Background(), &Request{URL: "https://api.example.com/v1/users"}); !IsDenied(err) {
		t.Fatalf("Do(v1) error = %v, want denied", err)
	}

	if _, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com/v1/%2e%2e/"},
	}); err == nil {
		t.Fatal("New() with encoded dot-segment prefix error = nil, want invalid config error")
	}
}

func TestIsPrivateIPCoversNonPublicRanges(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"0.0.0.0",
		"0.1.2.3",
		"::",
		"127.0.0.1",
		"::1",
		"::ffff:127.0.0.1",
		"::ffff:0.0.0.0",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"100.64.0.1",
		"100.127.255.254",
		"169.254.169.254",
		"192.0.0.8",
		"198.18.0.1",
		"240.0.0.1",
		"255.255.255.255",
		"224.0.0.1",
		"fe80::1",
		"fc00::1",
		"fd00:ec2::254",
		"fec0::1",
		"ff02::1",
		"::127.0.0.1",
		"64:ff9b::7f00:1",
		"64:ff9b::a9fe:a9fe",
		"64:ff9b:1::1",
		"2002:7f00:1::1",
		"2002:a9fe:a9fe::1",
		"100::1",
	} {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("ParseIP(%q) = nil", raw)
		}
		if !isPrivateIP(ip) {
			t.Errorf("isPrivateIP(%q) = false, want true", raw)
		}
	}

	for _, raw := range []string{
		"93.184.216.34",
		"8.8.8.8",
		"100.63.255.255",
		"100.128.0.1",
		"2606:4700:4700::1111",
		"64:ff9b::808:808",
		"2002:808:808::1",
	} {
		if isPrivateIP(net.ParseIP(raw)) {
			t.Errorf("isPrivateIP(%q) = true, want false", raw)
		}
	}
}

func TestClientBlocksUnspecifiedAndSharedAddressesLexically(t *testing.T) {
	t.Parallel()
	for _, target := range []string{
		"http://0.0.0.0/",
		"http://[::]/",
		"http://[::ffff:0.0.0.0]/",
		"http://100.64.0.1/",
	} {
		parsed, err := url.Parse(target)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", target, err)
		}
		doer := &recordingHTTPDoer{}
		client, err := New(&Config{
			AllowedURLPrefixes: []string{parsed.Scheme + "://" + parsed.Host},
			DenyPrivateRanges:  true,
		}, WithDoer(doer))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if _, err := client.Do(context.Background(), &Request{URL: target}); !IsDenied(err) {
			t.Errorf("Do(%q) error = %v, want denied error", target, err)
		}
		if sent := doer.sent(); len(sent) != 0 {
			t.Errorf("Do(%q) sent %v, want no requests", target, sent)
		}
	}
}

func TestClientBlocksUnspecifiedAddressesAfterDNSResolution(t *testing.T) {
	t.Parallel()
	for _, resolved := range []string{"0.0.0.0", "::", "100.64.0.1"} {
		doer := &recordingHTTPDoer{}
		client, err := New(&Config{
			AllowedURLPrefixes: []string{"https://api.example.com"},
			DenyPrivateRanges:  true,
		}, WithDoer(doer), WithResolver(resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(resolved)}}, nil
		})))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if _, err := client.Do(context.Background(), &Request{URL: "https://api.example.com/path"}); !IsDenied(err) {
			t.Errorf("resolved %s: Do() error = %v, want denied error", resolved, err)
		}
		if sent := doer.sent(); len(sent) != 0 {
			t.Errorf("resolved %s: sent %v, want no requests", resolved, sent)
		}
	}
}

func TestClientBlocksUnspecifiedAddressesDuringDial(t *testing.T) {
	t.Parallel()
	for _, resolved := range []string{"0.0.0.0", "::", "100.64.0.1"} {
		client, err := New(&Config{
			AllowedURLPrefixes: []string{"http://api.example.test:80"},
			DenyPrivateRanges:  true,
		}, WithResolver(&sequenceResolver{
			responses: [][]net.IPAddr{
				{{IP: net.ParseIP("93.184.216.34")}},
				{{IP: net.ParseIP(resolved)}},
			},
		}))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if _, err := client.Do(context.Background(), &Request{URL: "http://api.example.test:80/path"}); !IsDenied(err) {
			t.Errorf("dial resolved %s: Do() error = %v, want denied error", resolved, err)
		}
	}
}
