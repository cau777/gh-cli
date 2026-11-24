package list

import (
	"fmt"
	"regexp"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
)

var pullRequestSearchQualifierRE = regexp.MustCompile(`(?i)\b(?:is|type):(?:pr|pull-?request)\b`)

func listIssues(client *api.Client, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.IssuesAndTotalCount, error) {
	return api.ListIssuesREST(
		client,
		repo,
		filters.State,
		filters.Assignee,
		filters.Author,
		filters.Mention,
		filters.Labels,
		filters.Milestone,
		limit,
	)
}

func searchIssues(client *api.Client, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.IssuesAndTotalCount, error) {
	if pullRequestSearchQualifierRE.MatchString(filters.Search) {
		return nil, fmt.Errorf("cannot use pull request search qualifiers with `gh issue list`; use `gh pr list` instead")
	}

	filters.Repo = ghrepo.FullName(repo)
	filters.Entity = "issue"
	query := prShared.SearchQueryBuild(filters, true)
	return api.SearchIssuesREST(client, repo, query, "", limit)
}
