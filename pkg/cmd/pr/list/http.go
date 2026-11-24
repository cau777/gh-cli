package list

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
)

func shouldUseSearch(filters prShared.FilterOptions) bool {
	return filters.Draft != nil || filters.Author != "" || filters.Assignee != "" || filters.Search != "" || len(filters.Labels) > 0
}

func listPullRequests(httpClient *http.Client, detector fd.Detector, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.PullRequestAndTotalCount, error) {
	if shouldUseSearch(filters) {
		return searchPullRequests(httpClient, detector, repo, filters, limit)
	}

	return prShared.NewLister(httpClient).List(prShared.ListOptions{
		BaseRepo:     repo,
		LimitResults: limit,
		State:        filters.State,
		BaseBranch:   filters.BaseBranch,
		HeadBranch:   filters.HeadBranch,
		Fields:       filters.Fields,
	})
}

func searchPullRequests(httpClient *http.Client, _ fd.Detector, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.PullRequestAndTotalCount, error) {
	filters.Repo = ghrepo.FullName(repo)
	filters.Entity = "pr"
	q := prShared.SearchQueryBuild(filters, true)

	client := api.NewClientFromHTTP(httpClient)
	return api.SearchPullRequestsREST(client, repo, q, limit, filters.Fields)
}
