package featuredetection

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueFeatures(t *testing.T) {
	for _, host := range []string{"github.com", "stampname.ghe.com", "git.my.org"} {
		t.Run(host, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			detector := detector{host: host, httpClient: &http.Client{Transport: reg}}
			got, err := detector.IssueFeatures()
			require.NoError(t, err)
			assert.Equal(t, IssueFeatures{
				ApiActorsSupported:          false,
				IssueRelationshipsSupported: true,
			}, got)
		})
	}
}

func TestPullRequestFeatures(t *testing.T) {
	for _, host := range []string{"github.com", "git.my.org"} {
		t.Run(host, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			detector := detector{host: host, httpClient: &http.Client{Transport: reg}}
			got, err := detector.PullRequestFeatures()
			require.NoError(t, err)
			assert.Equal(t, PullRequestFeatures{
				MergeQueue:                     false,
				CheckRunAndStatusContextCounts: true,
				CheckRunEvent:                  true,
			}, got)
		})
	}
}

func TestRepositoryFeatures(t *testing.T) {
	for _, host := range []string{"github.com", "stampname.ghe.com", "git.my.org"} {
		t.Run(host, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			detector := detector{host: host, httpClient: &http.Client{Transport: reg}}
			got, err := detector.RepositoryFeatures()
			require.NoError(t, err)
			assert.Equal(t, allRepositoryFeatures, got)
		})
	}
}

func TestProjectV1Support(t *testing.T) {
	tests := []struct {
		name         string
		hostname     string
		httpStubs    func(*httpmock.Registry)
		wantFeatures gh.ProjectsV1Support
	}{
		{
			name:         "github.com",
			hostname:     "github.com",
			wantFeatures: gh.ProjectsV1Unsupported,
		},
		{
			name:         "ghec data residency (ghe.com)",
			hostname:     "stampname.ghe.com",
			wantFeatures: gh.ProjectsV1Unsupported,
		},
		{
			name:     "GHE 3.16.0",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.16.0"}`),
				)
			},
			wantFeatures: gh.ProjectsV1Supported,
		},
		{
			name:     "GHE 3.16.1",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.16.1"}`),
				)
			},
			wantFeatures: gh.ProjectsV1Supported,
		},
		{
			name:     "GHE 3.17",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.17.0"}`),
				)
			},
			wantFeatures: gh.ProjectsV1Unsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			httpClient := &http.Client{}
			httpmock.ReplaceTripper(httpClient, reg)

			detector := NewDetector(httpClient, tt.hostname)
			require.Equal(t, tt.wantFeatures, detector.ProjectsV1())
		})
	}
}

func TestSearchFeatures(t *testing.T) {
	tests := []struct {
		name         string
		hostname     string
		wantFeatures SearchFeatures
	}{
		{
			name:     "github.com",
			hostname: "github.com",
			wantFeatures: SearchFeatures{
				AdvancedIssueSearchAPI:      true,
				AdvancedIssueSearchAPIOptIn: false,
				SemanticSearch:              true,
				HybridSearch:                true,
			},
		},
		{
			name:     "ghec data residency (ghe.com)",
			hostname: "stampname.ghe.com",
			wantFeatures: SearchFeatures{
				AdvancedIssueSearchAPI:      true,
				AdvancedIssueSearchAPIOptIn: false,
				SemanticSearch:              true,
				HybridSearch:                true,
			},
		},
		{
			name:     "GHE",
			hostname: "git.my.org",
			wantFeatures: SearchFeatures{
				AdvancedIssueSearchAPI:      true,
				AdvancedIssueSearchAPIOptIn: false,
				SemanticSearch:              false,
				HybridSearch:                false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			detector := NewDetector(&http.Client{Transport: reg}, tt.hostname)
			features, err := detector.SearchFeatures()
			require.NoError(t, err)
			require.Equal(t, tt.wantFeatures, features)
		})
	}
}

func TestProjectFeatures(t *testing.T) {
	for _, host := range []string{"github.com", "stampname.ghe.com", "git.my.org"} {
		t.Run(host, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			detector := detector{host: host, httpClient: &http.Client{Transport: reg}}
			got, err := detector.ProjectFeatures()
			require.NoError(t, err)
			assert.Equal(t, allProjectFeatures, got)
		})
	}
}

func TestReleaseFeatures(t *testing.T) {
	for _, host := range []string{"github.com", "stampname.ghe.com", "git.my.org"} {
		t.Run(host, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			detector := NewDetector(&http.Client{Transport: reg}, host)
			features, err := detector.ReleaseFeatures()
			require.NoError(t, err)
			require.Equal(t, ReleaseFeatures{ImmutableReleases: true}, features)
		})
	}
}

func TestAuthFeatures(t *testing.T) {
	tests := []struct {
		hostname         string
		installedVersion string
		metaStatus       int
		wantExpiring     bool
		wantErr          bool
	}{
		{hostname: "github.com", wantExpiring: true},
		{hostname: "tenant.ghe.com", wantExpiring: true},
		{hostname: "github.example.com", installedVersion: "3.19.0"},
		{hostname: "github.example.com", installedVersion: "3.22.999"},
		{hostname: "github.example.com", installedVersion: "3.23.0", wantExpiring: true},
		{hostname: "github.example.com", installedVersion: "3.24.0", wantExpiring: true},
		{hostname: "github.example.com", installedVersion: "invalid", wantErr: true},
		{hostname: "github.example.com", wantErr: true},
		{hostname: "github.example.com", metaStatus: http.StatusServiceUnavailable, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.hostname+"/"+tt.installedVersion, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.installedVersion != "" || tt.wantErr {
				response := httpmock.JSONResponse(map[string]string{"installed_version": tt.installedVersion})
				if tt.metaStatus != 0 {
					response = httpmock.StatusStringResponse(tt.metaStatus, `{"message":"Service Unavailable"}`)
				}
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "api/v3/meta"), tt.hostname),
					response,
				)
			}

			features, err := NewDetector(&http.Client{Transport: reg}, tt.hostname).AuthFeatures()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantExpiring, features.ExpiringTokens)
		})
	}
}

func TestActionsFeatures(t *testing.T) {
	tests := []struct {
		name         string
		hostname     string
		httpStubs    func(*httpmock.Registry)
		wantFeatures ActionsFeatures
	}{
		{
			name:     "github.com, workflow dispatch run details supported",
			hostname: "github.com",
			wantFeatures: ActionsFeatures{
				DispatchRunDetails: true,
			},
		},
		{
			name:     "ghec data residency (ghe.com), workflow dispatch run details supported",
			hostname: "stampname.ghe.com",
			wantFeatures: ActionsFeatures{
				DispatchRunDetails: true,
			},
		},
		{
			name:     "GHE 3.20, workflow dispatch run details not supported",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.20.999"}`),
				)
			},
			wantFeatures: ActionsFeatures{
				DispatchRunDetails: false,
			},
		},
		{
			name:     "GHE 3.21, workflow dispatch run details supported",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.21.0"}`),
				)
			},
			wantFeatures: ActionsFeatures{
				DispatchRunDetails: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			httpClient := &http.Client{}
			httpmock.ReplaceTripper(httpClient, reg)

			detector := NewDetector(httpClient, tt.hostname)

			features, err := detector.ActionsFeatures()
			require.NoError(t, err)
			require.Equal(t, tt.wantFeatures, features)
		})
	}
}
