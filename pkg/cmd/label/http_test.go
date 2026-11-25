package label

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestLabelList_pagination(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := &http.Client{Transport: reg}

	reg.Register(
		httpmock.QueryMatcher("GET", "repos/OWNER/REPO/labels", map[string][]string{"per_page": {"100"}}),
		httpmock.WithHeader(
			httpmock.StringResponse(`[
				{
					"name": "bug",
					"color": "d73a4a",
					"description": "This is a bug label"
				}
			]`),
			"Link",
			`<https://api.github.com/repos/OWNER/REPO/labels?page=2&per_page=100>; rel="next"`,
		),
	)

	reg.Register(
		httpmock.QueryMatcher("GET", "repos/OWNER/REPO/labels", map[string][]string{"page": {"2"}, "per_page": {"100"}}),
		httpmock.StringResponse(`[
			{
				"name": "docs",
				"color": "ffa8da",
				"description": "This is a docs label"
			}
		]`),
	)

	repo := ghrepo.New("OWNER", "REPO")
	labels, totalCount, err := listLabels(client, repo, listQueryOptions{Limit: 10})
	assert.NoError(t, err)

	assert.Equal(t, 2, totalCount)
	assert.Equal(t, 2, len(labels))

	assert.Equal(t, "bug", labels[0].Name)
	assert.Equal(t, "d73a4a", labels[0].Color)
	assert.Equal(t, "This is a bug label", labels[0].Description)

	assert.Equal(t, "docs", labels[1].Name)
	assert.Equal(t, "ffa8da", labels[1].Color)
	assert.Equal(t, "This is a docs label", labels[1].Description)
}
