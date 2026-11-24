package api

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckUnsupportedIssueJSONFields(t *testing.T) {
	require.NoError(t, CheckUnsupportedIssueJSONFields([]string{"number", "title", "labels"}))

	for _, f := range UnsupportedIssueJSONFields {
		err := CheckUnsupportedIssueJSONFields([]string{"number", f})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotSupported)
	}
}

func TestSingleIssueREST(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues/123"),
		httpmock.StringResponse(`{
			"id": 9001,
			"node_id": "ISSUE_123",
			"number": 123,
			"title": "Bug title",
			"state": "closed",
			"state_reason": "not_planned",
			"body": "Body text",
			"html_url": "https://github.com/OWNER/REPO/issues/123",
			"locked": true,
			"active_lock_reason": "too heated",
			"comments": 1,
			"user": {"login": "octocat", "node_id": "U1", "type": "User"},
			"assignees": [{"login": "hubot", "node_id": "U2", "type": "Bot"}],
			"labels": [{"node_id": "L1", "name": "bug", "color": "d73a4a"}],
			"milestone": {"number": 4, "title": "v1.0"},
			"reactions": {"+1": 3, "heart": 2},
			"type": {"node_id": "IT1", "name": "Bug"},
			"sub_issues_summary": {"total": 2, "completed": 1, "percent_completed": 50}
		}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues/123/comments"),
		httpmock.StringResponse(`[
			{"id": 55, "node_id": "IC_55", "body": "First!", "user": {"login": "monalisa", "node_id": "U3"}, "author_association": "MEMBER", "html_url": "https://github.com/OWNER/REPO/issues/123#issuecomment-55"}
		]`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues/123/parent"),
		httpmock.StringResponse(`{"id": 100, "node_id": "P100", "number": 100, "title": "Parent", "state": "open", "html_url": "https://github.com/OWNER/REPO/issues/100"}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues/123/sub_issues"),
		httpmock.StringResponse(`[{"id": 124, "node_id": "S124", "number": 124, "title": "Child", "state": "closed", "html_url": "https://github.com/OWNER/REPO/issues/124"}]`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues/123/dependencies/blocked_by"),
		httpmock.StringResponse(`[{"id": 200, "node_id": "B200", "number": 200, "title": "Blocker", "state": "open", "html_url": "https://github.com/OWNER/REPO/issues/200"}]`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues/123/dependencies/blocking"),
		httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO"),
		httpmock.StringResponse(`{"id": 777, "node_id": "R1", "name": "REPO", "full_name": "OWNER/REPO", "permissions": {"push": true}}`),
	)

	client := NewClientFromHTTP(&http.Client{Transport: reg})
	repo := ghrepo.New("OWNER", "REPO")

	issue, err := SingleIssueREST(client, repo, 123, []string{
		"comments", "parent", "subIssues", "blockedBy", "blocking", "repository",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(9001), issue.DatabaseID)
	assert.Equal(t, "ISSUE_123", issue.ID)
	assert.Equal(t, "CLOSED", issue.State)
	assert.Equal(t, "NOT_PLANNED", issue.StateReason)
	assert.Equal(t, "TOO_HEATED", issue.ActiveLockReason)
	assert.True(t, issue.Locked)
	assert.Equal(t, "octocat", issue.Author.Login)
	assert.Equal(t, []string{"hubot"}, issue.Assignees.Logins())
	assert.Equal(t, []string{"bug"}, issue.Labels.Names())
	assert.Equal(t, "v1.0", issue.Milestone.Title)
	assert.Equal(t, "Bug", issue.IssueType.Name)
	assert.Equal(t, 2, len(issue.ReactionGroups))
	assert.Equal(t, 1, len(issue.Comments.Nodes))
	assert.Equal(t, int64(55), issue.Comments.Nodes[0].DatabaseID)
	require.NotNil(t, issue.Parent)
	assert.Equal(t, 100, issue.Parent.Number)
	assert.Equal(t, "OWNER/REPO", issue.Parent.Repository.NameWithOwner)
	require.Len(t, issue.SubIssues.Nodes, 1)
	require.Len(t, issue.BlockedBy.Nodes, 1)
	assert.Empty(t, issue.Blocking.Nodes)
	assert.Equal(t, int64(777), issue.RepositoryDatabaseID())
	assert.Equal(t, "WRITE", issue.RepositoryViewerPermission())
}

func TestSingleIssueREST_disabledIssues(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues/1"),
		httpmock.StatusStringResponse(410, `{"message":"Issues are disabled for this repo"}`),
	)

	client := NewClientFromHTTP(&http.Client{Transport: reg})
	_, err := SingleIssueREST(client, ghrepo.New("OWNER", "REPO"), 1, nil)
	assert.EqualError(t, err, "the 'OWNER/REPO' repository has disabled issues")
}

func TestListIssuesREST_filtersOutPRsAndPaginates(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues"),
		httpmock.WithHeader(
			httpmock.StringResponse(`[
				{"number": 10, "title": "PR item", "state": "open", "pull_request": {"url": "https://api.github.com/repos/OWNER/REPO/pulls/10"}},
				{"number": 9, "title": "Real issue 1", "state": "open", "labels": [{"name": "bug"}]}
			]`),
			"Link",
			`<https://api.github.com/repos/OWNER/REPO/issues?page=2>; rel="next"`,
		),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues"),
		httpmock.StringResponse(`[
			{"number": 8, "title": "Real issue 2", "state": "open"}
		]`),
	)

	client := NewClientFromHTTP(&http.Client{Transport: reg})
	res, err := ListIssuesREST(client, ghrepo.New("OWNER", "REPO"), "open", "", "", "", nil, "", 10)
	require.NoError(t, err)
	assert.Equal(t, 2, res.TotalCount)
	require.Len(t, res.Issues, 2)
	assert.Equal(t, 9, res.Issues[0].Number)
	assert.Equal(t, 8, res.Issues[1].Number)
}

func TestRepoIssueTypes_handles404Gracefully(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "orgs/OWNER/issue-types"),
		httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
	)

	client := NewClientFromHTTP(&http.Client{Transport: reg})
	types, err := RepoIssueTypes(client, ghrepo.New("OWNER", "REPO"))
	require.NoError(t, err)
	assert.Empty(t, types)
}
