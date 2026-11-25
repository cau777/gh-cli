package shared

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/capi"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

const uuidPattern = `[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`

var sessionIDRegexp = regexp.MustCompile(fmt.Sprintf("^%s$", uuidPattern))
var agentSessionURLRegexp = regexp.MustCompile(fmt.Sprintf("^/agent-sessions/(%s)$", uuidPattern))

func CapiClientFunc(f *cmdutil.Factory) func() (capi.CapiClient, error) {
	return func() (capi.CapiClient, error) {
		cfg, err := f.Config()
		if err != nil {
			return nil, err
		}

		httpClient, err := f.HttpClient()
		if err != nil {
			return nil, err
		}

		authCfg := cfg.Authentication()
		host, _ := authCfg.DefaultHost()

		cachedClient := api.NewCachedHTTPClient(httpClient, time.Minute*10)
		capiBaseURL, err := resolveCapiURL(cachedClient, host)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve Copilot API URL: %w", err)
		}

		// The CAPI client authenticates by setting the token header itself, which bypasses the base HTTP client's
		// auto-refresh. Pass authCfg so the CAPI transport can refresh an expiring token on each request.
		return capi.NewCAPIClient(httpClient, authCfg, host, capiBaseURL), nil
	}
}

// resolveCapiURL returns NotSupportedError because Copilot API endpoint discovery uses GraphQL.
func resolveCapiURL(_ *http.Client, _ string) (string, error) {
	return "", api.NotSupportedError("resolving Copilot API endpoints is not supported with REST")
}

func IsSessionID(s string) bool {
	return sessionIDRegexp.MatchString(s)
}

// ParseSessionIDFromURL parses session ID from a pull request's agent session
// URL, which is of the form:
//
//	`https://github.com/OWNER/REPO/pull/NUMBER/agent-sessions/SESSION-ID`
func ParseSessionIDFromURL(u string) (string, error) {
	_, _, rest, err := prShared.ParseURL(u)
	if err != nil {
		return "", err
	}

	match := agentSessionURLRegexp.FindStringSubmatch(rest)
	if match == nil {
		return "", errors.New("not a valid agent session URL")
	}
	return match[1], nil
}
