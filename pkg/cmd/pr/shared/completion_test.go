package shared

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/require"
)

func TestRequestableReviewersForCompletion(t *testing.T) {
	tests := []struct {
		name              string
		expectedReviewers []string
		httpStubs         func(*httpmock.Registry, *testing.T)
	}{
		{
			name:              "when users and teams are both available, both are listed",
			expectedReviewers: []string{"MonaLisa", "OWNER/core", "OWNER/robots", "hubot"},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login": "OWNER"}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/assignees"),
					httpmock.StringResponse(`[
						{ "login": "hubot", "node_id": "HUBOTID" },
						{ "login": "MonaLisa", "node_id": "MONAID" }
					]`))
				reg.Register(
					httpmock.REST("GET", "orgs/OWNER/teams"),
					httpmock.StringResponse(`[
						{ "slug": "core", "node_id": "COREID" },
						{ "slug": "robots", "node_id": "ROBOTID" }
					]`))
			},
		},
		{
			name:              "when users are available but teams aren't, users are listed",
			expectedReviewers: []string{"MonaLisa", "hubot"},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login": "OWNER"}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/assignees"),
					httpmock.StringResponse(`[
						{ "login": "hubot", "node_id": "HUBOTID" },
						{ "login": "MonaLisa", "node_id": "MONAID" }
					]`))
				reg.Register(
					httpmock.REST("GET", "orgs/OWNER/teams"),
					httpmock.StringResponse(`[]`))
			},
		},
		{
			name:              "when teams are available but users aren't, teams are listed",
			expectedReviewers: []string{"OWNER/core", "OWNER/robots"},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login": "OWNER"}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/assignees"),
					httpmock.StringResponse(`[]`))
				reg.Register(
					httpmock.REST("GET", "orgs/OWNER/teams"),
					httpmock.StringResponse(`[
						{ "slug": "core", "node_id": "COREID" },
						{ "slug": "robots", "node_id": "ROBOTID" }
					]`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg, t)
			}

			reviewers, err := RequestableReviewersForCompletion(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"))
			require.NoError(t, err)
			require.Equal(t, tt.expectedReviewers, reviewers)
		})
	}
}
