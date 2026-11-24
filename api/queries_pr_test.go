package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestBranchDeleteRemote(t *testing.T) {
	var tests = []struct {
		name        string
		branch      string
		httpStubs   func(*httpmock.Registry)
		expectError bool
	}{
		{
			name:   "success",
			branch: "owner/branch#123",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads%2Fowner%2Fbranch%23123"),
					httpmock.StatusStringResponse(204, ""))
			},
			expectError: false,
		},
		{
			name:   "error",
			branch: "my-branch",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads%2Fmy-branch"),
					httpmock.StatusStringResponse(500, `{"message": "oh no"}`))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(http)
			}

			client := newTestClient(http)
			repo, _ := ghrepo.FromFullName("OWNER/REPO")

			err := BranchDeleteRemote(client, repo, tt.branch)
			if (err != nil) != tt.expectError {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}

func Test_Logins(t *testing.T) {
	rr := ReviewRequests{}
	var tests = []struct {
		name             string
		requestedReviews string
		want             []string
	}{
		{
			name:             "no requested reviewers",
			requestedReviews: `{"nodes": []}`,
			want:             []string{},
		},
		{
			name: "user",
			requestedReviews: `{"nodes": [
				{
					"requestedreviewer": {
						"__typename": "User", "login": "testuser"
					}
				}
			]}`,
			want: []string{"testuser"},
		},
		{
			name: "team",
			requestedReviews: `{"nodes": [
				{
					"requestedreviewer": {
						"__typename": "Team",
						"name": "Test Team",
						"slug": "test-team",
						"organization": {"login": "myorg"}
					}
				}
			]}`,
			want: []string{"myorg/test-team"},
		},
		{
			name: "multiple users and teams",
			requestedReviews: `{"nodes": [
				{
					"requestedreviewer": {
						"__typename": "User", "login": "user1"
					}
				},
				{
					"requestedreviewer": {
						"__typename": "User", "login": "user2"
					}
				},
				{
					"requestedreviewer": {
						"__typename": "Team",
						"name": "Test Team",
						"slug": "test-team",
						"organization": {"login": "myorg"}
					}
				},
				{
					"requestedreviewer": {
						"__typename": "Team",
						"name": "Dev Team",
						"slug": "dev-team",
						"organization": {"login": "myorg"}
					}
				}
			]}`,
			want: []string{"user1", "user2", "myorg/test-team", "myorg/dev-team"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tt.requestedReviews), &rr)
			assert.NoError(t, err, "Failed to unmarshal json string as ReviewRequests")
			logins := rr.Logins()
			assert.Equal(t, tt.want, logins)
		})
	}
}

// TestPRRepositorySelectionMatchesStruct guards the seam between the two:
// a field added to the selection but not the struct is silently dropped, and
// one added to the struct but not the selection is silently zero.
func TestPRRepositorySelectionMatchesStruct(t *testing.T) {
	selection := PullRequestGraphQL([]string{"repository"})
	inner := strings.TrimSuffix(strings.TrimPrefix(selection, "repository{"), "}")
	selected := strings.Split(inner, ",")

	var declared []string
	repositoryType := reflect.TypeFor[PRRepository]()
	for field := range repositoryType.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		declared = append(declared, name)
	}

	assert.ElementsMatch(t, declared, selected)
}

func TestPullRequestRepositoryDatabaseID(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
		want int64
	}{
		{
			name: "the repository field was requested",
			pr:   PullRequest{Repository: &PRRepository{DatabaseID: 1234}},
			want: 1234,
		},
		{
			name: "the repository field was not requested",
			pr:   PullRequest{},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.pr.RepositoryDatabaseID())
		})
	}
}

func TestPullRequestRepositoryViewerPermission(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
		want string
	}{
		{
			name: "the repository field was requested",
			pr:   PullRequest{Repository: &PRRepository{ViewerPermission: "WRITE"}},
			want: "WRITE",
		},
		{
			name: "the repository field was not requested",
			pr:   PullRequest{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.pr.RepositoryViewerPermission())
		})
	}
}
