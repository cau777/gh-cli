package capi

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListLatestSessionsForViewer(t *testing.T) {
	sampleDateString := "2025-08-29T00:00:00Z"

	tests := []struct {
		name      string
		perPage   int
		limit     int
		httpStubs func(*testing.T, *httpmock.Registry)
		wantErr   string
		wantErrIs error
		wantOut   []*Session
	}{
		{
			name:    "zero limit",
			limit:   0,
			wantOut: nil,
		},
		{
			name:  "no sessions",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(`{"sessions":[]}`),
				)
			},
			wantOut: nil,
		},
		{
			name:  "API error",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StatusStringResponse(500, "{}"),
				)
			},
			wantErr: "failed to list sessions:",
		},
		{
			name:  "non-empty sessions return not supported on hydration",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sess1",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 2000,
									"created_at": "%[1]s",
									"premium_requests": 0.1
								}
							]
						}`,
						sampleDateString,
					)),
				)
			},
			wantErrIs: api.ErrNotSupported,
			wantErr:   "failed to fetch session resources:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			capiClient := NewCAPIClient(httpClient, stubAuthConfig{}, "github.com", "https://api.githubcopilot.com")

			if tt.perPage != 0 {
				last := defaultSessionsPerPage
				defaultSessionsPerPage = tt.perPage
				defer func() {
					defaultSessionsPerPage = last
				}()
			}

			sessions, err := capiClient.ListLatestSessionsForViewer(context.Background(), tt.limit)

			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, sessions)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOut, sessions)
		})
	}
}

func TestListSessionsByResourceIDRequiresResource(t *testing.T) {
	client := &CAPIClient{}

	_, err := client.ListSessionsByResourceID(context.Background(), "", 999, 0)
	assert.EqualError(t, err, "missing resource type/ID")
	_, err = client.ListSessionsByResourceID(context.Background(), "only-resource-type", 0, 0)
	assert.EqualError(t, err, "missing resource type/ID")
	_, err = client.ListSessionsByResourceID(context.Background(), "", 0, 0)
	assert.EqualError(t, err, "missing resource type/ID")
}

func TestListSessionsByResourceID(t *testing.T) {
	sampleDateString := "2025-08-29T07:00:00Z"

	resourceID := int64(999)
	resourceType := "pull"

	tests := []struct {
		name      string
		perPage   int
		limit     int
		httpStubs func(*testing.T, *httpmock.Registry)
		wantErr   string
		wantErrIs error
		wantOut   []*Session
	}{
		{
			name:    "zero limit",
			limit:   0,
			wantOut: nil,
		},
		{
			name:  "no sessions or no pull request",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/resource/pull/999"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(404, "{}"),
				)
			},
			wantErr: "failed to list sessions",
		},
		{
			name:  "API error",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/resource/pull/999"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(500, "{}"),
				)
			},
			wantErr: "failed to list sessions:",
		},
		{
			name:  "non-empty sessions return not supported on hydration",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/resource/pull/999"), "api.githubcopilot.com"),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sess1",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 2000,
									"created_at": "%[1]s",
									"premium_requests": 0.1
								}
							]
						}`,
						sampleDateString,
					)),
				)
			},
			wantErrIs: api.ErrNotSupported,
			wantErr:   "failed to fetch session resources:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			capiClient := NewCAPIClient(httpClient, stubAuthConfig{}, "github.com", "https://api.githubcopilot.com")

			if tt.perPage != 0 {
				last := defaultSessionsPerPage
				defaultSessionsPerPage = tt.perPage
				defer func() {
					defaultSessionsPerPage = last
				}()
			}

			sessions, err := capiClient.ListSessionsByResourceID(context.Background(), resourceType, resourceID, tt.limit)

			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, sessions)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOut, sessions)
		})
	}
}

func TestGetSessionRequiresID(t *testing.T) {
	client := &CAPIClient{}

	_, err := client.GetSession(context.Background(), "")
	assert.EqualError(t, err, "missing session ID")
}

func TestGetSession(t *testing.T) {
	sampleDateString := "2025-08-29T00:00:00Z"

	tests := []struct {
		name      string
		httpStubs func(*testing.T, *httpmock.Registry)
		wantErr   string
		wantErrIs error
		wantOut   *Session
	}{
		{
			name: "session not found",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(404, "{}"),
				)
			},
			wantErrIs: ErrSessionNotFound,
			wantErr:   "not found",
		},
		{
			name: "API error",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(500, "some error"),
				)
			},
			wantErr: "failed to get session:",
		},
		{
			name: "invalid JSON response",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(200, ""),
				)
			},
			wantErr: "failed to decode session response: EOF",
		},
		{
			name: "non-empty session returns not supported on hydration",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"id": "some-uuid",
							"name": "Build artifacts",
							"user_id": 1,
							"agent_id": 2,
							"logs": "",
							"state": "completed",
							"owner_id": 10,
							"repo_id": 1000,
							"resource_type": "pull",
							"resource_id": 2000,
							"created_at": "%[1]s",
							"premium_requests": 0.1
						}`,
						sampleDateString,
					)),
				)
			},
			wantErrIs: api.ErrNotSupported,
			wantErr:   "failed to fetch session resources:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			capiClient := NewCAPIClient(httpClient, stubAuthConfig{}, "github.com", "https://api.githubcopilot.com")

			session, err := capiClient.GetSession(context.Background(), "some-uuid")

			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
			}

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, session)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOut, session)
		})
	}
}

func TestGetPullRequestDatabaseID(t *testing.T) {
	tests := []struct {
		name           string
		httpStubs      func(*testing.T, *httpmock.Registry)
		wantErr        string
		wantDatabaseID int64
		wantURL        string
	}{
		{
			name: "REST error",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "repos/OWNER/REPO/pulls/42"), "api.github.com"),
					httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
				)
			},
			wantErr: "HTTP 404",
		},
		{
			name: "success",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "repos/OWNER/REPO/pulls/42"), "api.github.com"),
					httpmock.StringResponse(`{"id": 999, "html_url": "some-url"}`),
				)
			},
			wantDatabaseID: 999,
			wantURL:        "some-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			capiClient := NewCAPIClient(httpClient, stubAuthConfig{}, "github.com", "https://api.githubcopilot.com")

			databaseID, url, err := capiClient.GetPullRequestDatabaseID(context.Background(), "github.com", "OWNER", "REPO", 42)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Zero(t, databaseID)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantDatabaseID, databaseID)
			require.Equal(t, tt.wantURL, url)
		})
	}
}
