package create

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_repoCreate(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		input    repoCreateInput
		stubs    func(t *testing.T, r *httpmock.Registry)
		wantErr  bool
		errMsg   string
		wantRepo string
	}{
		{
			name:     "create personal repository",
			hostname: "github.com",
			input: repoCreateInput{
				Name:             "winter-foods",
				Description:      "roasted chestnuts",
				HomepageURL:      "http://example.com",
				Visibility:       "public",
				HasIssuesEnabled: true,
				HasWikiEnabled:   true,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("POST", "user/repos"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"name":        "winter-foods",
							"description": "roasted chestnuts",
							"homepage":    "http://example.com",
							"private":     false,
							"has_issues":  true,
							"has_wiki":    true,
						}, payload)
					}),
				)
			},
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name:     "create Enterprise repository",
			hostname: "example.com",
			input: repoCreateInput{
				Name:             "winter-foods",
				Description:      "roasted chestnuts",
				HomepageURL:      "http://example.com",
				Visibility:       "public",
				HasIssuesEnabled: true,
				HasWikiEnabled:   true,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("POST", "api/v3/user/repos"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"name":        "winter-foods",
							"description": "roasted chestnuts",
							"homepage":    "http://example.com",
							"private":     false,
							"has_issues":  true,
							"has_wiki":    true,
						}, payload)
					}),
				)
			},
			wantRepo: "https://example.com/OWNER/REPO",
		},
		{
			name:     "create in organization",
			hostname: "github.com",
			input: repoCreateInput{
				Name:             "crisps",
				Visibility:       "internal",
				OwnerLogin:       "snacks-inc",
				HasIssuesEnabled: true,
				HasWikiEnabled:   true,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "users/snacks-inc"),
					httpmock.StringResponse(`{ "node_id": "ORGID", "type": "Organization" }`))
				r.Register(
					httpmock.REST("POST", "orgs/snacks-inc/repos"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"name":       "crisps",
							"private":    false,
							"visibility": "internal",
							"has_issues": true,
							"has_wiki":   true,
						}, payload)
					}),
				)
			},
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name:     "create for team",
			hostname: "github.com",
			input: repoCreateInput{
				Name:             "crisps",
				Visibility:       "internal",
				OwnerLogin:       "snacks-inc",
				TeamSlug:         "munchies",
				HasIssuesEnabled: true,
				HasWikiEnabled:   true,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "orgs/snacks-inc/teams/munchies"),
					httpmock.StringResponse(`{ "node_id": "TEAMID", "id": 1234, "organization": {"node_id": "ORGID"} }`))
				r.Register(
					httpmock.REST("POST", "orgs/snacks-inc/repos"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"name":       "crisps",
							"private":    false,
							"visibility": "internal",
							"team_id":    float64(1234),
							"has_issues": true,
							"has_wiki":   true,
						}, payload)
					}),
				)
			},
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name:     "create personal repo from template repo",
			hostname: "github.com",
			input: repoCreateInput{
				Name:                 "gen-project",
				Description:          "my generated project",
				Visibility:           "private",
				TemplateRepository:   ghrepo.NewWithHost("TPLOWNER", "TPLREPO", "github.com"),
				TemplateRepositoryID: "TPLID",
				HasIssuesEnabled:     true,
				HasWikiEnabled:       true,
				IncludeAllBranches:   false,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"OWNER"}`))
				r.Register(
					httpmock.REST("POST", "repos/TPLOWNER/TPLREPO/generate"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"owner":                "OWNER",
							"name":                 "gen-project",
							"description":          "my generated project",
							"private":              true,
							"include_all_branches": false,
						}, payload)
					}),
				)
			},
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name:     "create personal repo from template repo, and disable wiki",
			hostname: "github.com",
			input: repoCreateInput{
				Name:                 "gen-project",
				Description:          "my generated project",
				Visibility:           "private",
				TemplateRepository:   ghrepo.NewWithHost("TPLOWNER", "TPLREPO", "github.com"),
				TemplateRepositoryID: "TPLID",
				HasIssuesEnabled:     true,
				HasWikiEnabled:       false,
				IncludeAllBranches:   false,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"OWNER"}`))
				r.Register(
					httpmock.REST("POST", "repos/TPLOWNER/TPLREPO/generate"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"owner":                "OWNER",
							"name":                 "gen-project",
							"description":          "my generated project",
							"private":              true,
							"include_all_branches": false,
						}, payload)
					}),
				)
				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"has_issues": true,
							"has_wiki":   false,
						}, payload)
					}),
				)
			},
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name:     "create personal repo from template repo, and disable issues",
			hostname: "github.com",
			input: repoCreateInput{
				Name:                 "gen-project",
				Description:          "my generated project",
				Visibility:           "private",
				TemplateRepository:   ghrepo.NewWithHost("TPLOWNER", "TPLREPO", "github.com"),
				TemplateRepositoryID: "TPLID",
				HasIssuesEnabled:     false,
				HasWikiEnabled:       true,
				IncludeAllBranches:   false,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"OWNER"}`))
				r.Register(
					httpmock.REST("POST", "repos/TPLOWNER/TPLREPO/generate"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"owner":                "OWNER",
							"name":                 "gen-project",
							"description":          "my generated project",
							"private":              true,
							"include_all_branches": false,
						}, payload)
					}),
				)
				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"has_issues": false,
							"has_wiki":   true,
						}, payload)
					}),
				)
			},
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name:     "create personal repo from template repo, and disable both wiki and issues",
			hostname: "github.com",
			input: repoCreateInput{
				Name:                 "gen-project",
				Description:          "my generated project",
				Visibility:           "private",
				TemplateRepository:   ghrepo.NewWithHost("TPLOWNER", "TPLREPO", "github.com"),
				TemplateRepositoryID: "TPLID",
				HasIssuesEnabled:     false,
				HasWikiEnabled:       false,
				IncludeAllBranches:   false,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"OWNER"}`))
				r.Register(
					httpmock.REST("POST", "repos/TPLOWNER/TPLREPO/generate"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"owner":                "OWNER",
							"name":                 "gen-project",
							"description":          "my generated project",
							"private":              true,
							"include_all_branches": false,
						}, payload)
					}),
				)
				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"has_issues": false,
							"has_wiki":   false,
						}, payload)
					}),
				)
			},
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name:     "create personal repo from template repo, and set homepage url",
			hostname: "github.com",
			input: repoCreateInput{
				Name:                 "gen-project",
				Description:          "my generated project",
				Visibility:           "private",
				TemplateRepository:   ghrepo.NewWithHost("TPLOWNER", "TPLREPO", "github.com"),
				TemplateRepositoryID: "TPLID",
				HasIssuesEnabled:     true,
				HasWikiEnabled:       true,
				IncludeAllBranches:   false,
				HomepageURL:          "https://example.com",
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"OWNER"}`))
				r.Register(
					httpmock.REST("POST", "repos/TPLOWNER/TPLREPO/generate"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"owner":                "OWNER",
							"name":                 "gen-project",
							"description":          "my generated project",
							"private":              true,
							"include_all_branches": false,
						}, payload)
					}),
				)
				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"has_issues": true,
							"has_wiki":   true,
							"homepage":   "https://example.com",
						}, payload)
					}),
				)
			},
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name:     "create org repo from template repo",
			hostname: "github.com",
			input: repoCreateInput{
				Name:                 "gen-project",
				Description:          "my generated project",
				Visibility:           "private",
				OwnerLogin:           "myorg",
				TemplateRepository:   ghrepo.NewWithHost("TPLOWNER", "TPLREPO", "github.com"),
				TemplateRepositoryID: "TPLID",
				HasIssuesEnabled:     true,
				HasWikiEnabled:       true,
				IncludeAllBranches:   false,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "users/myorg"),
					httpmock.StringResponse(`{ "node_id": "ORGID", "type": "Organization" }`))
				r.Register(
					httpmock.REST("POST", "repos/TPLOWNER/TPLREPO/generate"),
					httpmock.RESTPayload(201, `{"name":"REPO","owner":{"login":"OWNER"},"html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"owner":                "myorg",
							"name":                 "gen-project",
							"description":          "my generated project",
							"private":              true,
							"include_all_branches": false,
						}, payload)
					}),
				)
			},
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name:     "create org repo from template repo as internal is not supported",
			hostname: "github.com",
			input: repoCreateInput{
				Name:                 "gen-project",
				Description:          "my generated project",
				Visibility:           "internal",
				OwnerLogin:           "myorg",
				TemplateRepository:   ghrepo.NewWithHost("TPLOWNER", "TPLREPO", "github.com"),
				TemplateRepositoryID: "TPLID",
				HasIssuesEnabled:     true,
				HasWikiEnabled:       true,
				IncludeAllBranches:   false,
			},
			wantErr: true,
			errMsg:  api.ErrNotSupported.Error(),
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "users/myorg"),
					httpmock.StringResponse(`{ "node_id": "ORGID", "type": "Organization" }`))
			},
		},
		{
			name:     "create with license and gitignore",
			hostname: "github.com",
			input: repoCreateInput{
				Name:              "crisps",
				Visibility:        "private",
				LicenseTemplate:   "lgpl-3.0",
				GitIgnoreTemplate: "Go",
				HasIssuesEnabled:  true,
				HasWikiEnabled:    true,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("POST", "user/repos"),
					httpmock.RESTPayload(201, `{"name":"crisps", "owner":{"login": "snacks-inc"}, "html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"name":               "crisps",
							"private":            true,
							"gitignore_template": "Go",
							"license_template":   "lgpl-3.0",
							"has_issues":         true,
							"has_wiki":           true,
						}, payload)
					}))
			},
			wantRepo: "https://github.com/snacks-inc/crisps",
		},
		{
			name:     "create with README",
			hostname: "github.com",
			input: repoCreateInput{
				Name:       "crisps",
				InitReadme: true,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("POST", "user/repos"),
					httpmock.RESTPayload(201, `{"name":"crisps", "owner":{"login": "snacks-inc"}, "html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"name":       "crisps",
							"private":    false,
							"has_issues": false,
							"has_wiki":   false,
							"auto_init":  true,
						}, payload)
					}))
			},
			wantRepo: "https://github.com/snacks-inc/crisps",
		},
		{
			name:     "create with license and gitignore on Enterprise",
			hostname: "example.com",
			input: repoCreateInput{
				Name:              "crisps",
				Visibility:        "private",
				LicenseTemplate:   "lgpl-3.0",
				GitIgnoreTemplate: "Go",
				HasIssuesEnabled:  true,
				HasWikiEnabled:    true,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("POST", "api/v3/user/repos"),
					httpmock.RESTPayload(201, `{"name":"crisps", "owner":{"login": "snacks-inc"}, "html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"name":               "crisps",
							"private":            true,
							"gitignore_template": "Go",
							"license_template":   "lgpl-3.0",
							"has_issues":         true,
							"has_wiki":           true,
						}, payload)
					}))
			},
			wantRepo: "https://example.com/snacks-inc/crisps",
		},
		{
			name:     "create with license and gitignore in org",
			hostname: "github.com",
			input: repoCreateInput{
				Name:              "crisps",
				Visibility:        "INTERNAL",
				OwnerLogin:        "snacks-inc",
				LicenseTemplate:   "lgpl-3.0",
				GitIgnoreTemplate: "Go",
				HasIssuesEnabled:  true,
				HasWikiEnabled:    true,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "users/snacks-inc"),
					httpmock.StringResponse(`{ "node_id": "ORGID", "type": "Organization" }`))
				r.Register(
					httpmock.REST("POST", "orgs/snacks-inc/repos"),
					httpmock.RESTPayload(201, `{"name":"crisps", "owner":{"login": "snacks-inc"}, "html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"name":               "crisps",
							"private":            false,
							"visibility":         "internal",
							"gitignore_template": "Go",
							"license_template":   "lgpl-3.0",
							"has_issues":         true,
							"has_wiki":           true,
						}, payload)
					}))
			},
			wantRepo: "https://github.com/snacks-inc/crisps",
		},
		{
			name:     "create with license and gitignore for team",
			hostname: "github.com",
			input: repoCreateInput{
				Name:              "crisps",
				Visibility:        "internal",
				OwnerLogin:        "snacks-inc",
				TeamSlug:          "munchies",
				LicenseTemplate:   "lgpl-3.0",
				GitIgnoreTemplate: "Go",
				HasIssuesEnabled:  true,
				HasWikiEnabled:    true,
			},
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "orgs/snacks-inc/teams/munchies"),
					httpmock.StringResponse(`{ "node_id": "TEAMID", "id": 1234, "organization": {"node_id": "ORGID"} }`))
				r.Register(
					httpmock.REST("POST", "orgs/snacks-inc/repos"),
					httpmock.RESTPayload(201, `{"name":"crisps", "owner":{"login": "snacks-inc"}, "html_url":"the://URL"}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{
							"name":               "crisps",
							"private":            false,
							"visibility":         "internal",
							"gitignore_template": "Go",
							"license_template":   "lgpl-3.0",
							"team_id":            float64(1234),
							"has_issues":         true,
							"has_wiki":           true,
						}, payload)
					}))
			},
			wantRepo: "https://github.com/snacks-inc/crisps",
		},
		{
			name:     "create personal repository but try to set it as 'internal'",
			hostname: "github.com",
			input: repoCreateInput{
				Name:        "winter-foods",
				Description: "roasted chestnuts",
				HomepageURL: "http://example.com",
				Visibility:  "internal",
				OwnerLogin:  "OWNER",
			},
			wantErr: true,
			errMsg:  "internal repositories can only be created within an organization",
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "users/OWNER"),
					httpmock.StringResponse(`{ "node_id": "1234", "type": "Not-Org" }`))
				r.Exclude(
					t,
					httpmock.REST("POST", "user/repos"),
				)
			},
		},
		{
			name:     "create personal repository with README but try to set it as 'internal'",
			hostname: "github.com",
			input: repoCreateInput{
				Name:        "winter-foods",
				Description: "roasted chestnuts",
				HomepageURL: "http://example.com",
				Visibility:  "internal",
				OwnerLogin:  "OWNER",
				InitReadme:  true,
			},
			wantErr: true,
			errMsg:  "internal repositories can only be created within an organization",
			stubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "users/OWNER"),
					httpmock.StringResponse(`{ "node_id": "1234", "type": "Not-Org" }`))
				r.Exclude(
					t,
					httpmock.REST("POST", "user/repos"),
				)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(t, reg)
			httpClient := &http.Client{Transport: reg}
			r, err := repoCreate(httpClient, tt.hostname, tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.ErrorContains(t, err, tt.errMsg)
				}
				return
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantRepo, ghrepo.GenerateRepoURL(r, ""))
		})
	}
}
