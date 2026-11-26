package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

const (
	apiVersion      = "X-GitHub-Api-Version"
	apiVersionValue = "2022-11-28"
	authorization   = "Authorization"
	cacheTTL        = "X-GH-CACHE-TTL"
	userAgent       = "User-Agent"
)

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

var nextPageRewriteRules = []struct {
	reqRE  *regexp.Regexp
	nextRE *regexp.Regexp
}{
	{
		reqRE:  regexp.MustCompile(`^(.*)/repos/([^/]+)/([^/]+)(/.*)?$`),
		nextRE: regexp.MustCompile(`^(.*)/repositories/([0-9]+)(/.*)?$`),
	},
	{
		reqRE:  regexp.MustCompile(`^(.*)/orgs/([^/]+)(/.*)?$`),
		nextRE: regexp.MustCompile(`^(.*)/organizations/([0-9]+)(/.*)?$`),
	},
	{
		reqRE:  regexp.MustCompile(`^(.*)/users/([^/]+)(/.*)?$`),
		nextRE: regexp.MustCompile(`^(.*)/user/([0-9]+)(/.*)?$`),
	},
}

// RewriteNextPageURL rewrites GitHub canonical ID paths in a pagination next
// link (such as /repositories/<id>/..., /organizations/<id>/..., or
// /user/<id>/...) back to the named path from reqURL (such as
// /repos/<owner>/<repo>/..., /orgs/<org>/..., or /users/<user>/...) when the
// prefix and trailing path segments match. Query parameters from nextLink are
// preserved unchanged.
func RewriteNextPageURL(reqURL *url.URL, nextLink string) string {
	if reqURL == nil || nextLink == "" {
		return nextLink
	}

	nextU, err := url.Parse(nextLink)
	if err != nil {
		return nextLink
	}

	for _, rule := range nextPageRewriteRules {
		reqMatch := rule.reqRE.FindStringSubmatch(reqURL.Path)
		nextMatch := rule.nextRE.FindStringSubmatch(nextU.Path)
		if reqMatch == nil || nextMatch == nil {
			continue
		}
		if reqMatch[1] == nextMatch[1] && reqMatch[len(reqMatch)-1] == nextMatch[len(nextMatch)-1] {
			nextU.Path = reqURL.Path
			nextU.RawPath = reqURL.RawPath
			return nextU.String()
		}
	}

	return nextLink
}

func NewClientFromHTTP(httpClient *http.Client) *Client {
	client := &Client{http: httpClient}
	return client
}

type Client struct {
	http *http.Client
}

func (c *Client) HTTP() *http.Client {
	return c.http
}

type HTTPError struct {
	*ghAPI.HTTPError
	scopesSuggestion string
}

func (err HTTPError) ScopesSuggestion() string {
	return err.scopesSuggestion
}

// RequestOption configures a single request made through Request or RequestWithContext.
type RequestOption func(*requestConfig)

// requestConfig accumulates the effect of the RequestOptions applied to one request.
type requestConfig struct {
	headers           map[string]string
	endpointScopes    string
	noFollowRedirects bool
}

// WithHeader sets a header on the request, taking precedence over any header of the same name
// that the underlying transport would otherwise supply.
func WithHeader(name, value string) RequestOption {
	return func(cfg *requestConfig) {
		cfg.headers[name] = value
	}
}

// WithoutFollowingRedirects stops the request from following redirects. When a redirect is
// encountered, the request fails with an HTTPError carrying the redirect status code and headers
// rather than following the redirect. This matters for requests where following a redirect
// silently changes the meaning of the request: Go's default policy converts methods other than
// GET or HEAD (e.g. DELETE) into a GET when it follows a 301/302/303, so a caller deleting a renamed
// resource would receive a success response while having deleted nothing.
//
// This option applies to the REST request surface (Request and RequestWithContext).
func WithoutFollowingRedirects() RequestOption {
	return func(cfg *requestConfig) {
		cfg.noFollowRedirects = true
	}
}

// WithEndpointScopes adds OAuth scopes to a 4xx error as if the server endpoint had returned them.
// This improves error messaging for endpoints that do not explicitly list the scopes they need, and
// is the Request equivalent of calling EndpointNeedsScopes on a raw response.
func WithEndpointScopes(scopes string) RequestOption {
	return func(cfg *requestConfig) {
		cfg.endpointScopes = scopes
	}
}

func newRequestConfig(opts []RequestOption) requestConfig {
	cfg := requestConfig{headers: map[string]string{}}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// REST performs a REST request and parses the response.
func (c Client) REST(hostname string, method string, p string, body io.Reader, data any) error {
	opts := clientOptions(hostname, c.http.Transport)
	restClient, err := ghAPI.NewRESTClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(restClient.Do(method, p, body, data))
}

func (c Client) RESTWithNext(hostname string, method string, p string, body io.Reader, data any) (string, error) {
	opts := clientOptions(hostname, c.http.Transport)
	restClient, err := ghAPI.NewRESTClient(opts)
	if err != nil {
		return "", err
	}

	resp, err := restClient.Request(method, p, body)
	if err != nil {
		return "", handleResponse(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return "", nil
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return "", err
	}

	var reqURL *url.URL
	if resp.Request != nil {
		reqURL = resp.Request.URL
	}
	var next string
	for _, m := range linkRE.FindAllStringSubmatch(resp.Header.Get("Link"), -1) {
		if len(m) > 2 && m[2] == "next" {
			next = RewriteNextPageURL(reqURL, m[1])
		}
	}

	return next, nil
}

// Request performs a REST request and returns the response for the caller to consume.
//
// See RequestWithContext for the full semantics.
func (c Client) Request(hostname string, method string, p string, body io.Reader, opts ...RequestOption) (*http.Response, error) {
	return c.RequestWithContext(context.Background(), hostname, method, p, body, opts...)
}

// RequestWithContext performs a REST request and returns the response for the caller to consume,
// rather than decoding it into a receiver as REST does. It exists for call sites that need access
// to the response itself: streaming bodies, non-JSON bodies, response headers, or the status code
// of a successful response.
//
// p may be a path relative to the host's REST endpoint, or an absolute URL. Prefer a relative path.
// Absolute URLs are requested as given, which is correct for URLs supplied by the API itself, such
// as asset and upload URLs, but means they do not benefit from host-level endpoint resolution.
//
// Responses outside the 2xx range are returned as an HTTPError and their body is closed. On success
// the response is returned unread and the caller is responsible for closing its body.
func (c Client) RequestWithContext(ctx context.Context, hostname string, method string, p string, body io.Reader, opts ...RequestOption) (*http.Response, error) {
	cfg := newRequestConfig(opts)
	clientOpts := clientOptions(hostname, c.http.Transport)
	maps.Copy(clientOpts.Headers, cfg.headers)
	if cfg.noFollowRedirects {
		clientOpts.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	restClient, err := ghAPI.NewRESTClient(clientOpts)
	if err != nil {
		return nil, err
	}

	resp, err := restClient.RequestWithContext(ctx, method, p, body)
	if err != nil {
		return nil, handleRequestError(err, cfg.endpointScopes)
	}

	return resp, nil
}

// DoRequest sends a request that the caller has already built, and returns the response for the
// caller to consume. It applies the same error handling as Request.
//
// Prefer Request. DoRequest exists for the few call sites that must control something Request
// cannot express. That is either a field of the request which is neither method, path, body nor
// header, such as ContentLength and GetBody when uploading a release asset, or a property of the
// client itself, such as CheckRedirect. Request delegates to go-gh, which builds a client of its
// own from the transport alone, so a redirect policy set on the client passed to
// NewClientFromHTTP only survives when the request is sent here.
//
// Because the caller supplies the URL, requests sent this way do not benefit from host-level
// endpoint resolution, so it is only appropriate for absolute URLs.
//
// Only the endpoint scopes of any RequestOption are applied; headers are the caller's own to set
// on the request they built.
//
// Responses outside the 2xx range are returned as an HTTPError and their body is closed. On success
// the response is returned unread and the caller is responsible for closing its body.
func (c Client) DoRequest(req *http.Request, opts ...RequestOption) (*http.Response, error) {
	cfg := newRequestConfig(opts)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		defer resp.Body.Close()
		if cfg.endpointScopes != "" {
			EndpointNeedsScopes(resp, cfg.endpointScopes)
		}
		return nil, HandleHTTPError(resp)
	}

	return resp, nil
}

// HandleHTTPError parses a http.Response into a HTTPError.
//
// The caller is responsible to close the response body stream.
func HandleHTTPError(resp *http.Response) error {
	return handleResponse(ghAPI.HandleHTTPError(resp))
}

// UnexpectedStatusError reports a successful response whose status the caller did not expect.
//
// The request methods on Client convert every non-2xx response into an HTTPError, so a call site
// that requires one specific status can only ever be surprised by a different 2xx. Passing such a
// response to HandleHTTPError would ask an error parser to explain a success, so this is used
// instead. The caller remains responsible for closing the response body stream.
func UnexpectedStatusError(resp *http.Response) error {
	if resp.Request != nil {
		return fmt.Errorf("unexpected HTTP %d for %s %s", resp.StatusCode, resp.Request.Method, resp.Request.URL)
	}
	return fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
}

// handleRequestError converts a request error into an HTTPError, first adding any
// endpoint scopes to the error's headers so that the scopes suggestion generated by handleResponse
// accounts for them. This is the error-path equivalent of calling EndpointNeedsScopes on a response
// before HandleHTTPError, which call sites can no longer do when the response never reaches them.
func handleRequestError(err error, endpointScopes string) error {
	if endpointScopes != "" {
		if restErr, ok := errors.AsType[*ghAPI.HTTPError](err); ok && restErr.Headers != nil && restErr.StatusCode >= 400 && restErr.StatusCode < 500 {
			oldScopes := restErr.Headers.Get("X-Accepted-Oauth-Scopes")
			restErr.Headers.Set("X-Accepted-Oauth-Scopes", fmt.Sprintf("%s, %s", oldScopes, endpointScopes))
		}
	}
	return handleResponse(err)
}

// handleResponse takes a ghAPI.HTTPError and converts it into an HTTPError.
func handleResponse(err error) error {
	if err == nil {
		return nil
	}

	if restErr, ok := errors.AsType[*ghAPI.HTTPError](err); ok {
		return HTTPError{
			HTTPError: restErr,
			scopesSuggestion: generateScopesSuggestion(restErr.StatusCode,
				restErr.Headers.Get("X-Accepted-Oauth-Scopes"),
				restErr.Headers.Get("X-Oauth-Scopes"),
				restErr.RequestURL.Hostname()),
		}
	}

	return err
}

// ScopesSuggestion is an error messaging utility that prints the suggestion to request additional OAuth
// scopes in case a server response indicates that there are missing scopes.
func ScopesSuggestion(resp *http.Response) string {
	return generateScopesSuggestion(resp.StatusCode,
		resp.Header.Get("X-Accepted-Oauth-Scopes"),
		resp.Header.Get("X-Oauth-Scopes"),
		resp.Request.URL.Hostname())
}

// EndpointNeedsScopes adds additional OAuth scopes to an HTTP response as if they were returned from the
// server endpoint. This improves HTTP 4xx error messaging for endpoints that don't explicitly list the
// OAuth scopes they need.
func EndpointNeedsScopes(resp *http.Response, s string) {
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		oldScopes := resp.Header.Get("X-Accepted-Oauth-Scopes")
		resp.Header.Set("X-Accepted-Oauth-Scopes", fmt.Sprintf("%s, %s", oldScopes, s))
	}
}

func generateScopesSuggestion(statusCode int, endpointNeedsScopes, tokenHasScopes, hostname string) string {
	if statusCode < 400 || statusCode > 499 || statusCode == 422 {
		return ""
	}

	if tokenHasScopes == "" {
		return ""
	}

	gotScopes := map[string]struct{}{}
	for s := range strings.SplitSeq(tokenHasScopes, ",") {
		s = strings.TrimSpace(s)
		gotScopes[s] = struct{}{}

		// Certain scopes may be grouped under a single "top-level" scope. The following branch
		// statements include these grouped/implied scopes when the top-level scope is encountered.
		// See https://docs.github.com/en/developers/apps/building-oauth-apps/scopes-for-oauth-apps.
		if s == "repo" {
			gotScopes["repo:status"] = struct{}{}
			gotScopes["repo_deployment"] = struct{}{}
			gotScopes["public_repo"] = struct{}{}
			gotScopes["repo:invite"] = struct{}{}
			gotScopes["security_events"] = struct{}{}
		} else if s == "user" {
			gotScopes["read:user"] = struct{}{}
			gotScopes["user:email"] = struct{}{}
			gotScopes["user:follow"] = struct{}{}
		} else if s == "codespace" {
			gotScopes["codespace:secrets"] = struct{}{}
		} else if after, ok := strings.CutPrefix(s, "admin:"); ok {
			gotScopes["read:"+after] = struct{}{}
			gotScopes["write:"+strings.TrimPrefix(s, "admin:")] = struct{}{}
		} else if after, ok := strings.CutPrefix(s, "write:"); ok {
			gotScopes["read:"+after] = struct{}{}
		}
	}

	for s := range strings.SplitSeq(endpointNeedsScopes, ",") {
		s = strings.TrimSpace(s)
		if _, gotScope := gotScopes[s]; s == "" || gotScope {
			continue
		}
		return fmt.Sprintf(
			"This API operation needs the %[1]q scope. To request it, run:  gh auth refresh -h %[2]s -s %[1]s",
			s,
			ghauth.NormalizeHostname(hostname),
		)
	}

	return ""
}

func clientOptions(hostname string, transport http.RoundTripper) ghAPI.ClientOptions {
	// AuthToken, and Headers are being handled by transport,
	// so let go-gh know that it does not need to resolve them.
	opts := ghAPI.ClientOptions{
		AuthToken: "none",
		Headers: map[string]string{
			authorization: "",
			apiVersion:    apiVersionValue,
		},
		Host:               hostname,
		SkipDefaultHeaders: true,
		Transport:          transport,
		LogIgnoreEnv:       true,
	}
	return opts
}
