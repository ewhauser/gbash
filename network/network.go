package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"
)

type Method string

const (
	MethodGet     Method = "GET"
	MethodHead    Method = "HEAD"
	MethodPost    Method = "POST"
	MethodPut     Method = "PUT"
	MethodDelete  Method = "DELETE"
	MethodPatch   Method = "PATCH"
	MethodOptions Method = "OPTIONS"
)

const (
	defaultMaxRedirects    = 20
	defaultTimeout         = 30 * time.Second
	defaultMaxResponseSize = 10 << 20
)

var defaultAllowedMethods = []Method{MethodGet, MethodHead}

type Config struct {
	AllowedURLPrefixes []string
	AllowedMethods     []Method
	MaxRedirects       int
	Timeout            time.Duration
	MaxResponseBytes   int64
	DenyPrivateRanges  bool
}

type Request struct {
	Method          string
	URL             string
	Headers         map[string]string
	Body            []byte
	FollowRedirects bool
	Timeout         time.Duration
}

type Response struct {
	StatusCode int
	Status     string
	Headers    map[string]string
	Body       []byte
	URL        string
}

type Client interface {
	Do(context.Context, *Request) (*Response, error)
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type Option func(*HTTPClient)

type HTTPClient struct {
	cfg      resolvedConfig
	doer     HTTPDoer
	resolver Resolver
}

type resolvedConfig struct {
	allowedURLPrefixes []allowListEntry
	allowedMethods     map[Method]struct{}
	maxRedirects       int
	timeout            time.Duration
	maxResponseBytes   int64
	denyPrivateRanges  bool
}

type allowListEntry struct {
	origin     string
	pathPrefix string
}

type deniedMarker interface {
	networkDenied()
}

type AccessDeniedError struct {
	URL    string
	Reason string
}

func (e *AccessDeniedError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("network access denied: %s", redactForError(e.URL))
	}
	return fmt.Sprintf("network access denied: %s: %s", e.Reason, redactForError(e.URL))
}

func (*AccessDeniedError) networkDenied() {}

type MethodNotAllowedError struct {
	Method string
}

func (e *MethodNotAllowedError) Error() string {
	return fmt.Sprintf("network access denied: method %q not allowed", e.Method)
}

func (*MethodNotAllowedError) networkDenied() {}

type RedirectNotAllowedError struct {
	URL string
}

func (e *RedirectNotAllowedError) Error() string {
	return fmt.Sprintf("network redirect denied: %s", redactForError(e.URL))
}

func (*RedirectNotAllowedError) networkDenied() {}

type TooManyRedirectsError struct {
	MaxRedirects int
}

func (e *TooManyRedirectsError) Error() string {
	return fmt.Sprintf("too many redirects (max %d)", e.MaxRedirects)
}

type ResponseTooLargeError struct {
	MaxBytes int64
}

func (e *ResponseTooLargeError) Error() string {
	return fmt.Sprintf("response body too large (max %d bytes)", e.MaxBytes)
}

type InvalidConfigError struct {
	Problems []string
}

func (e *InvalidConfigError) Error() string {
	return fmt.Sprintf("invalid network config:\n%s", strings.Join(e.Problems, "\n"))
}

func IsDenied(err error) bool {
	var denied deniedMarker
	return errors.As(err, &denied)
}

func WithDoer(doer HTTPDoer) Option {
	return func(client *HTTPClient) {
		client.doer = doer
	}
}

func WithResolver(resolver Resolver) Option {
	return func(client *HTTPClient) {
		client.resolver = resolver
	}
}

func New(cfg *Config, opts ...Option) (*HTTPClient, error) {
	resolved, err := resolveConfig(cfg)
	if err != nil {
		return nil, err
	}

	client := &HTTPClient{
		cfg:      resolved,
		resolver: net.DefaultResolver,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	if client.doer == nil {
		client.doer = client.defaultDoer()
	}
	return client, nil
}

func (c *HTTPClient) defaultDoer() HTTPDoer {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           c.dialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (c *HTTPClient) dialContext(ctx context.Context, networkName, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	ips, err := c.resolveDialIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	if c.cfg.denyPrivateRanges {
		if slices.ContainsFunc(ips, isPrivateIP) {
			return nil, &AccessDeniedError{URL: host, Reason: "host resolves to private or loopback IP"}
		}
	}

	dialer := net.Dialer{KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, networkName, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func (c *HTTPClient) resolveDialIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}

	addresses, err := c.lookupIPAddrs(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if address.IP != nil {
			ips = append(ips, address.IP)
		}
	}
	if len(ips) == 0 {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return ips, nil
}

func (c *HTTPClient) lookupIPAddrs(ctx context.Context, host string) ([]net.IPAddr, error) {
	resolver := c.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return resolver.LookupIPAddr(ctx, host)
}

func resolveConfig(cfg *Config) (resolvedConfig, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	entries, problems := normalizeAllowList(cfg.AllowedURLPrefixes)
	if len(problems) > 0 {
		return resolvedConfig{}, &InvalidConfigError{Problems: problems}
	}
	if len(entries) == 0 {
		return resolvedConfig{}, &InvalidConfigError{Problems: []string{"at least one allowed URL prefix is required"}}
	}

	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = defaultAllowedMethods
	}
	allowedMethods := make(map[Method]struct{}, len(methods))
	for _, method := range methods {
		method = Method(strings.ToUpper(strings.TrimSpace(string(method))))
		if method == "" {
			continue
		}
		allowedMethods[method] = struct{}{}
	}
	if len(allowedMethods) == 0 {
		return resolvedConfig{}, &InvalidConfigError{Problems: []string{"at least one allowed HTTP method is required"}}
	}

	maxRedirects := cfg.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = defaultMaxRedirects
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxResponseBytes := cfg.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseSize
	}

	return resolvedConfig{
		allowedURLPrefixes: entries,
		allowedMethods:     allowedMethods,
		maxRedirects:       maxRedirects,
		timeout:            timeout,
		maxResponseBytes:   maxResponseBytes,
		denyPrivateRanges:  cfg.DenyPrivateRanges,
	}, nil
}

func (c *HTTPClient) Do(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		req = &Request{}
	}

	method := Method(strings.ToUpper(strings.TrimSpace(req.Method)))
	if method == "" {
		method = MethodGet
	}
	if _, ok := c.cfg.allowedMethods[method]; !ok {
		return nil, &MethodNotAllowedError{Method: string(method)}
	}

	currentURL := strings.TrimSpace(req.URL)
	if currentURL == "" {
		return nil, &AccessDeniedError{URL: currentURL, Reason: "missing URL"}
	}

	timeout := c.cfg.timeout
	if req.Timeout > 0 && req.Timeout < timeout {
		timeout = req.Timeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	redirects := 0
	body := req.Body
	dropBodyHeaders := false
	var originalOrigin string
	for {
		target, err := c.allowedURL(ctx, currentURL)
		if err != nil {
			return nil, err
		}
		currentURL = target.String()
		if redirects == 0 {
			originalOrigin = urlOrigin(target)
		}
		// Like curl, credentials only follow redirects that stay on the
		// origin (scheme, host and port) of the initial request.
		sameOrigin := urlOrigin(target) == originalOrigin

		httpReq, err := http.NewRequestWithContext(ctx, string(method), currentURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		// Send exactly the normalized URL that passed the allowlist check.
		httpReq.URL = target
		for name, value := range req.Headers {
			if !sameOrigin && (isCredentialHeader(name) || IsSensitiveName(name)) {
				continue
			}
			if dropBodyHeaders && isBodyHeader(name) {
				continue
			}
			httpReq.Header.Set(name, value)
		}

		resp, err := c.doer.Do(httpReq)
		if err != nil {
			return nil, err
		}
		if isRedirect(resp.StatusCode) && req.FollowRedirects {
			location := strings.TrimSpace(resp.Header.Get("Location"))
			if location == "" {
				return c.readResponse(currentURL, resp)
			}
			_ = resp.Body.Close()

			nextURL, err := resolveRedirectURL(currentURL, location)
			if err != nil {
				return nil, err
			}
			if err := c.checkURLAllowed(ctx, nextURL); err != nil {
				return nil, &RedirectNotAllowedError{URL: nextURL}
			}
			redirects++
			if redirects > c.cfg.maxRedirects {
				return nil, &TooManyRedirectsError{MaxRedirects: c.cfg.maxRedirects}
			}
			if redirectSwitchesToGet(resp.StatusCode, method) {
				method = MethodGet
				body = nil
				dropBodyHeaders = true
				if _, ok := c.cfg.allowedMethods[method]; !ok {
					return nil, &MethodNotAllowedError{Method: string(method)}
				}
			}
			currentURL = nextURL
			continue
		}

		return c.readResponse(currentURL, resp)
	}
}

func (c *HTTPClient) readResponse(requestURL string, resp *http.Response) (*Response, error) {
	defer func() { _ = resp.Body.Close() }()

	if c.cfg.maxResponseBytes > 0 && resp.ContentLength > c.cfg.maxResponseBytes {
		return nil, &ResponseTooLargeError{MaxBytes: c.cfg.maxResponseBytes}
	}

	reader := io.Reader(resp.Body)
	if c.cfg.maxResponseBytes > 0 {
		reader = io.LimitReader(resp.Body, c.cfg.maxResponseBytes+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if c.cfg.maxResponseBytes > 0 && int64(len(body)) > c.cfg.maxResponseBytes {
		return nil, &ResponseTooLargeError{MaxBytes: c.cfg.maxResponseBytes}
	}

	headers := make(map[string]string, len(resp.Header))
	for name, values := range resp.Header {
		headers[name] = strings.Join(values, ", ")
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    headers,
		Body:       body,
		URL:        requestURL,
	}, nil
}

func (c *HTTPClient) checkURLAllowed(ctx context.Context, rawURL string) error {
	_, err := c.allowedURL(ctx, rawURL)
	return err
}

// allowedURL parses rawURL, removes dot segments from its path, and checks the
// normalized URL against the policy. Callers must send the returned URL so the
// request on the wire matches what was checked.
func (c *HTTPClient) allowedURL(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, &AccessDeniedError{URL: rawURL, Reason: "invalid URL"}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, &AccessDeniedError{URL: rawURL, Reason: "only http and https are allowed"}
	}
	if parsed.Host == "" || parsed.Opaque != "" {
		return nil, &AccessDeniedError{URL: rawURL, Reason: "missing host"}
	}
	if err := normalizeURLPath(parsed); err != nil {
		return nil, &AccessDeniedError{URL: rawURL, Reason: err.Error()}
	}
	if !urlAllowed(parsed, c.cfg.allowedURLPrefixes) {
		return nil, &AccessDeniedError{URL: rawURL, Reason: "URL not in allowlist"}
	}
	if c.cfg.denyPrivateRanges {
		if err := c.checkPrivateHost(ctx, parsed); err != nil {
			return nil, err
		}
	}
	return parsed, nil
}

// normalizeURLPath applies RFC 3986 remove_dot_segments to the escaped path of
// u in place. Paths that still contain a segment a server could interpret as
// "." or ".." after percent-decoding, backslash separators, or ";" path
// parameters are rejected rather than guessed at.
func normalizeURLPath(u *url.URL) error {
	escaped := removeDotSegments(u.EscapedPath())
	if hasAmbiguousDotSegment(escaped) {
		return errors.New("ambiguous dot segment in URL path")
	}
	decoded, err := url.PathUnescape(escaped)
	if err != nil {
		return errors.New("invalid URL path escape")
	}
	u.Path = decoded
	u.RawPath = escaped
	return nil
}

func removeDotSegments(escaped string) string {
	if !strings.HasPrefix(escaped, "/") {
		return escaped
	}
	segments := strings.Split(escaped[1:], "/")
	out := make([]string, 0, len(segments))
	for i, segment := range segments {
		last := i == len(segments)-1
		switch segment {
		case ".":
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, segment)
			continue
		}
		if last {
			out = append(out, "")
		}
	}
	return "/" + strings.Join(out, "/")
}

func hasAmbiguousDotSegment(escaped string) bool {
	if hasDoubleEncodedSeparator(escaped) {
		return true
	}
	for segment := range strings.SplitSeq(escaped, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return true
		}
		for part := range strings.FieldsFuncSeq(decoded, func(r rune) bool { return r == '/' || r == '\\' }) {
			part, _, _ = strings.Cut(part, ";")
			if part == "." || part == ".." {
				return true
			}
		}
	}
	return false
}

func (c *HTTPClient) checkPrivateHost(ctx context.Context, parsed *url.URL) error {
	host := parsed.Hostname()
	if isPrivateHostname(host) {
		return &AccessDeniedError{URL: parsed.String(), Reason: "private or loopback host blocked"}
	}

	if _, err := netip.ParseAddr(host); err == nil {
		return nil
	}

	addresses, err := c.lookupIPAddrs(ctx, host)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && (dnsErr.IsNotFound || dnsErr.Err == "no such host") {
			return nil
		}
		return &AccessDeniedError{URL: parsed.String(), Reason: "DNS resolution failed for private range check"}
	}
	for _, address := range addresses {
		if isPrivateIP(address.IP) {
			return &AccessDeniedError{URL: parsed.String(), Reason: "host resolves to private or loopback IP"}
		}
	}
	return nil
}

func normalizeAllowList(entries []string) (normalized []allowListEntry, problems []string) {
	if len(entries) == 0 {
		return nil, nil
	}

	out := make([]allowListEntry, 0, len(entries))
	problems = make([]string, 0)
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parsed, err := url.Parse(entry)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			problems = append(problems, fmt.Sprintf("invalid allowlist URL %q", entry))
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			problems = append(problems, fmt.Sprintf("allowlist URL must use http or https: %q", entry))
			continue
		}
		if parsed.User != nil {
			problems = append(problems, fmt.Sprintf("allowlist URL must not include userinfo: %q", entry))
			continue
		}
		if err := normalizeURLPath(parsed); err != nil {
			problems = append(problems, fmt.Sprintf("allowlist URL has %s: %q", err, entry))
			continue
		}
		normalized := allowListEntry{
			origin:     parsed.Scheme + "://" + parsed.Host,
			pathPrefix: parsed.EscapedPath(),
		}
		if normalized.pathPrefix == "" {
			normalized.pathPrefix = "/"
		}
		key := normalized.origin + normalized.pathPrefix
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out, problems
}

func urlAllowed(target *url.URL, entries []allowListEntry) bool {
	origin := target.Scheme + "://" + target.Host
	pathname := target.EscapedPath()
	if pathname == "" {
		pathname = "/"
	}

	for _, entry := range entries {
		if origin != entry.origin {
			continue
		}
		if allowListPathMatches(pathname, entry.pathPrefix) {
			return true
		}
	}
	return false
}

func allowListPathMatches(pathname, prefix string) bool {
	if prefix == "" || prefix == "/" {
		return true
	}
	if pathname == prefix {
		return true
	}
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(pathname, prefix)
	}
	return strings.HasPrefix(pathname, prefix+"/")
}

func resolveRedirectURL(currentURL, location string) (string, error) {
	base, err := url.Parse(currentURL)
	if err != nil {
		return "", err
	}
	next, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(next).String(), nil
}

// redirectSwitchesToGet reports whether following a redirect with statusCode
// rewrites the request to a body-less GET. 301, 302 and 303 do so for every
// method except GET and HEAD; 307 and 308 preserve the method and body.
func redirectSwitchesToGet(statusCode int, method Method) bool {
	switch statusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther:
		return method != MethodGet && method != MethodHead
	default:
		return false
	}
}

// urlOrigin returns the scheme, host and port of u with default ports made
// explicit, so that "https://h" and "https://h:443" compare equal.
func urlOrigin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

// hasDoubleEncodedSeparator reports whether escaped contains "%25" followed
// by an encoded ".", "/" or "\\", which a server that decodes twice would
// turn into a dot segment or separator.
func hasDoubleEncodedSeparator(escaped string) bool {
	lower := strings.ToLower(escaped)
	for {
		i := strings.Index(lower, "%25")
		if i < 0 {
			return false
		}
		rest := lower[i+3:]
		if strings.HasPrefix(rest, "2e") || strings.HasPrefix(rest, "2f") || strings.HasPrefix(rest, "5c") {
			return true
		}
		lower = rest
	}
}

// redactForError hides secrets in a URL included in error text.
func redactForError(rawURL string) string {
	if redacted, ok := RedactURL(rawURL); ok {
		return redacted
	}
	return rawURL
}

// isCredentialHeader reports whether a request header carries credentials
// that must not be forwarded to a different origin on redirect.
func isCredentialHeader(name string) bool {
	switch http.CanonicalHeaderKey(strings.TrimSpace(name)) {
	case "Authorization", "Proxy-Authorization", "Cookie", "Cookie2":
		return true
	default:
		return false
	}
}

// isBodyHeader reports whether a request header describes the request body
// and must be dropped once a redirect discards that body.
func isBodyHeader(name string) bool {
	switch http.CanonicalHeaderKey(strings.TrimSpace(name)) {
	case "Content-Type", "Content-Length", "Content-Encoding", "Content-Language", "Content-Location", "Transfer-Encoding":
		return true
	default:
		return false
	}
}

func isRedirect(statusCode int) bool {
	switch statusCode {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func isPrivateHostname(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return isPrivateAddr(addr)
}

// nonPublicPrefixes lists special-purpose ranges that are not globally
// reachable and are not already covered by the netip classification helpers
// used in isPrivateAddr.
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"; 0.0.0.0 reaches localhost
	netip.MustParsePrefix("100.64.0.0/10"),   // shared address space (CGNAT)
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, including 255.255.255.255
	netip.MustParsePrefix("::/96"),           // unspecified and deprecated IPv4-compatible
	netip.MustParsePrefix("::ffff:0:0:0/96"), // IPv4-translated
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use IPv4/IPv6 translation
	netip.MustParsePrefix("100::/64"),        // discard-only
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("fec0::/10"),       // deprecated site-local
}

var (
	nat64Prefix  = netip.MustParsePrefix("64:ff9b::/96")
	sixToFourNet = netip.MustParsePrefix("2002::/16")
)

func isPrivateIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	return isPrivateAddr(addr)
}

func isPrivateAddr(addr netip.Addr) bool {
	addr = addr.Unmap().WithZone("")
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsMulticast() {
		return true
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	// Translation prefixes embed an IPv4 destination; classify that instead.
	if nat64Prefix.Contains(addr) {
		raw := addr.As16()
		return isPrivateAddr(netip.AddrFrom4([4]byte(raw[12:])))
	}
	if sixToFourNet.Contains(addr) {
		raw := addr.As16()
		return isPrivateAddr(netip.AddrFrom4([4]byte(raw[2:6])))
	}
	return false
}
