package api

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateProjectV2Items(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)
	repo, _ := ghrepo.FromFullName("OWNER/REPO")

	err := UpdateProjectV2Items(client, repo, nil, nil)
	assert.NoError(t, err)

	addProjectItems := map[string]string{"project1": "item1"}
	err = UpdateProjectV2Items(client, repo, addProjectItems, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotSupported)
}

func TestProjectsV2ItemsForIssue(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)
	repo, _ := ghrepo.FromFullName("OWNER/REPO")
	issue := &Issue{Number: 1}

	err := ProjectsV2ItemsForIssue(client, repo, issue)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotSupported)
	assert.Empty(t, issue.ProjectItems.Nodes)
}

func TestProjectsV2ItemsForPullRequest(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)
	repo, _ := ghrepo.FromFullName("OWNER/REPO")
	pr := &PullRequest{Number: 1}

	err := ProjectsV2ItemsForPullRequest(client, repo, pr)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotSupported)
	assert.Empty(t, pr.ProjectItems.Nodes)
}

func TestProjectsV2IgnorableError(t *testing.T) {
	var tests = []struct {
		name      string
		err       error
		expectOut bool
	}{
		{
			name:      "not supported error",
			err:       NotSupportedError("Projects V2 items are not supported with REST"),
			expectOut: true,
		},
		{
			name:      "read scope error",
			err:       errors.New("field requires one of the following scopes: ['read:project']"),
			expectOut: true,
		},
		{
			name:      "repository projectsV2 field error",
			err:       errors.New("Field 'projectsV2' doesn't exist on type 'Repository'"),
			expectOut: true,
		},
		{
			name:      "organization projectsV2 field error",
			err:       errors.New("Field 'projectsV2' doesn't exist on type 'Organization'"),
			expectOut: true,
		},
		{
			name:      "issue projectItems field error",
			err:       errors.New("Field 'projectItems' doesn't exist on type 'Issue'"),
			expectOut: true,
		},
		{
			name:      "pullRequest projectItems field error",
			err:       errors.New("Field 'projectItems' doesn't exist on type 'PullRequest'"),
			expectOut: true,
		},
		{
			name:      "resource not accessible by integration",
			err:       errors.New("Resource not accessible by integration"),
			expectOut: true,
		},
		{
			name:      "resource not accessible by personal access token",
			err:       errors.New("Resource not accessible by personal access token"),
			expectOut: true,
		},
		{
			name:      "other error",
			err:       errors.New("some other error message"),
			expectOut: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ProjectsV2IgnorableError(tt.err)
			assert.Equal(t, tt.expectOut, out)
		})
	}
}
