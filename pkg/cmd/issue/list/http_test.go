package list

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestIssueList(t *testing.T) {
	reg := &httpmock.Registry{}
	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	client := api.NewClientFromHTTP(httpClient)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues"),
		httpmock.WithHeader(
			httpmock.StringResponse(`[]`),
			"Link",
			`<https://api.github.com/repos/OWNER/REPO/issues?page=2&per_page=100&state=open>; rel="next"`,
		),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues"),
		httpmock.StringResponse(`[]`),
	)

	repo, _ := ghrepo.FromFullName("OWNER/REPO")
	filters := prShared.FilterOptions{
		Entity: "issue",
		State:  "open",
	}
	_, err := listIssues(client, repo, filters, 251)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reg.Requests) != 2 {
		t.Fatalf("expected 2 HTTP requests, seen %d", len(reg.Requests))
	}
	assert.Equal(t, url.Values{"per_page": {"100"}, "state": {"open"}}, reg.Requests[0].URL.Query())
	assert.Equal(t, "2", reg.Requests[1].URL.Query().Get("page"))
}

func TestIssueList_pagination(t *testing.T) {
	reg := &httpmock.Registry{}
	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	client := api.NewClientFromHTTP(httpClient)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues"),
		httpmock.WithHeader(
			httpmock.StringResponse(`[
				{
					"number": 1,
					"title": "issue1",
					"labels": [ { "name": "bug" } ],
					"assignees": [ { "login": "user1" } ]
				}
			]`),
			"Link",
			`<https://api.github.com/repos/OWNER/REPO/issues?page=2>; rel="next"`,
		),
	)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues"),
		httpmock.StringResponse(`[
			{
				"number": 2,
				"title": "issue2",
				"labels": [ { "name": "enhancement" } ],
				"assignees": [ { "login": "user2" } ]
			}
		]`),
	)

	repo := ghrepo.New("OWNER", "REPO")
	res, err := listIssues(client, repo, prShared.FilterOptions{}, 0)
	if err != nil {
		t.Fatalf("IssueList() error = %v", err)
	}

	assert.Equal(t, 2, res.TotalCount)
	assert.Equal(t, 2, len(res.Issues))

	getLabels := func(i api.Issue) []string {
		var labels []string
		for _, l := range i.Labels.Nodes {
			labels = append(labels, l.Name)
		}
		return labels
	}
	getAssignees := func(i api.Issue) []string {
		var logins []string
		for _, u := range i.Assignees.Nodes {
			logins = append(logins, u.Login)
		}
		return logins
	}

	assert.Equal(t, []string{"bug"}, getLabels(res.Issues[0]))
	assert.Equal(t, []string{"user1"}, getAssignees(res.Issues[0]))
	assert.Equal(t, []string{"enhancement"}, getLabels(res.Issues[1]))
	assert.Equal(t, []string{"user2"}, getAssignees(res.Issues[1]))
}

func TestSearchIssuesRESTQuery(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"q":        {"repo:OWNER/REPO state:open type:issue"},
			"per_page": {"30"},
		}),
		httpmock.StringResponse(`{"total_count": 0, "items": []}`),
	)

	httpClient := &http.Client{Transport: reg}
	client := api.NewClientFromHTTP(httpClient)

	_, err := searchIssues(client, ghrepo.New("OWNER", "REPO"), prShared.FilterOptions{State: "open"}, 30)
	assert.NoError(t, err)
}

func TestSearchIssues_rejectsPullRequestQualifiers(t *testing.T) {
	tests := []struct {
		name   string
		search string
	}{
		{
			name:   "is:pr",
			search: "is:pr",
		},
		{
			name:   "type:pr",
			search: "type:pr",
		},
		{
			name:   "type:pull-request",
			search: "type:pull-request",
		},
		{
			name:   "type:pullrequest",
			search: "type:pullrequest",
		},
		{
			name:   "case-insensitive is:PR",
			search: "is:PR",
		},
		{
			name:   "case-insensitive TYPE:Pull-Request",
			search: "TYPE:Pull-Request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}
			client := api.NewClientFromHTTP(httpClient)

			_, err := searchIssues(
				client,
				ghrepo.New("OWNER", "REPO"),
				prShared.FilterOptions{Search: tt.search},
				30,
			)

			assert.EqualError(t, err, "cannot use pull request search qualifiers with `gh issue list`; use `gh pr list` instead")
			assert.Len(t, reg.Requests, 0)
		})
	}
}
