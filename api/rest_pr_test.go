package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeLatestReviews(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	t3 := t2.Add(time.Hour)

	reviews := []PullRequestReview{
		{ID: "1", Author: CommentAuthor{Login: "alice"}, State: "APPROVED", SubmittedAt: &t1},
		{ID: "2", Author: CommentAuthor{Login: "alice"}, State: "COMMENTED", SubmittedAt: &t2},
		{ID: "3", Author: CommentAuthor{Login: "bob"}, State: "COMMENTED", SubmittedAt: &t1},
		{ID: "4", Author: CommentAuthor{Login: "bob"}, State: "CHANGES_REQUESTED", SubmittedAt: &t2},
		{ID: "5", Author: CommentAuthor{Login: "carol"}, State: "PENDING", SubmittedAt: &t3},
		{ID: "6", Author: CommentAuthor{Login: "dave"}, State: "COMMENTED", SubmittedAt: &t3},
	}

	latest := ComputeLatestReviews(reviews)
	require.Len(t, latest.Nodes, 3)
	assert.Equal(t, 3, latest.TotalCount)
	assert.Equal(t, "APPROVED", latest.Nodes[0].State)
	assert.Equal(t, "alice", latest.Nodes[0].Author.Login)
	assert.Equal(t, "CHANGES_REQUESTED", latest.Nodes[1].State)
	assert.Equal(t, "bob", latest.Nodes[1].Author.Login)
	assert.Equal(t, "COMMENTED", latest.Nodes[2].State)
	assert.Equal(t, "dave", latest.Nodes[2].Author.Login)
}

func TestFetchStatusChecksREST(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/commits/abc123/check-runs"),
		httpmock.StringResponse(`{
			"total_count": 1,
			"check_runs": [
				{
					"id": 1,
					"name": "build",
					"status": "completed",
					"conclusion": "success",
					"html_url": "https://github.com/OWNER/REPO/runs/1",
					"check_suite": {"id": 99}
				}
			]
		}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/commits/abc123/status"),
		httpmock.StringResponse(`{
			"total_count": 1,
			"statuses": [
				{
					"context": "ci/legacy",
					"state": "pending",
					"description": "waiting",
					"target_url": "https://ci.example.com/1"
				}
			]
		}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
		httpmock.StringResponse(`{
			"workflow_runs": [
				{
					"id": 50,
					"name": "CI",
					"event": "pull_request",
					"check_suite_id": 99
				}
			]
		}`),
	)

	client := NewClientFromHTTP(&http.Client{Transport: reg})
	repo := ghrepo.New("OWNER", "REPO")

	contexts, err := FetchStatusChecksREST(client, repo, "abc123", true)
	require.NoError(t, err)
	require.Len(t, contexts, 2)

	assert.Equal(t, "CheckRun", contexts[0].TypeName)
	assert.Equal(t, "build", contexts[0].Name)
	assert.Equal(t, "COMPLETED", contexts[0].Status)
	assert.Equal(t, CheckConclusionStateSuccess, contexts[0].Conclusion)
	assert.Equal(t, "CI", contexts[0].CheckSuite.WorkflowRun.Workflow.Name)
	assert.Equal(t, "pull_request", contexts[0].CheckSuite.WorkflowRun.Event)

	assert.Equal(t, "StatusContext", contexts[1].TypeName)
	assert.Equal(t, "ci/legacy", contexts[1].Context)
	assert.Equal(t, StatusStatePending, contexts[1].State)
	assert.Equal(t, "waiting", contexts[1].Description)
}

func TestRestPullRequestToPullRequest(t *testing.T) {
	mergedAt := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	mergeable := true
	raw := restPullRequest{
		ID:                  42,
		NodeID:              "PR_42",
		Number:              12,
		Title:               "Fix bug",
		State:               "closed",
		Merged:              true,
		MergedAt:            &mergedAt,
		HTMLURL:             "https://github.com/OWNER/REPO/pull/12",
		Mergeable:           &mergeable,
		MergeableState:      "clean",
		MaintainerCanModify: true,
		Commits:             3,
		User:                &restUser{NodeID: "U1", Login: "monalisa", Name: "Mona"},
		Head: &restPullBranch{
			Ref:  "fix",
			SHA:  "deadbeef",
			User: &restUser{Login: "forker"},
			Repo: &restRepo{FullName: "forker/REPO", NodeID: "R2"},
		},
		Base: &restPullBranch{
			Ref:  "main",
			SHA:  "cafebabe",
			Repo: &restRepo{ID: 7, FullName: "OWNER/REPO", NodeID: "R1"},
		},
	}

	pr := raw.toPullRequest(ghrepo.New("OWNER", "REPO"))
	assert.Equal(t, "PR_42", pr.ID)
	assert.Equal(t, "42", pr.FullDatabaseID)
	assert.Equal(t, "MERGED", pr.State)
	assert.True(t, pr.Closed)
	assert.True(t, pr.IsCrossRepository)
	assert.Equal(t, "forker:fix", pr.HeadLabel())
	assert.Equal(t, PullRequestMergeableMergeable, pr.Mergeable)
	assert.Equal(t, "CLEAN", pr.MergeStateStatus)
	assert.Equal(t, 3, pr.Commits.TotalCount)
	assert.Equal(t, int64(7), pr.RepositoryDatabaseID())
}
