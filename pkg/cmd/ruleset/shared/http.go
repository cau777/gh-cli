package shared

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type RulesetResponse struct {
	Level struct {
		Rulesets struct {
			TotalCount int
			Nodes      []RulesetGraphQL
			PageInfo   struct {
				HasNextPage bool
				EndCursor   string
			}
		}
	}
}

type RulesetList struct {
	TotalCount int
	Rulesets   []RulesetGraphQL
}

func ListRepoRulesets(httpClient *http.Client, repo ghrepo.Interface, limit int, includeParents bool) (*RulesetList, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "rulesets")
	if err != nil {
		return nil, err
	}
	return listRulesets(httpClient, u, limit, includeParents, repo.RepoHost())
}

func ListOrgRulesets(httpClient *http.Client, orgLogin string, limit int, host string, includeParents bool) (*RulesetList, error) {
	u, err := safeurl.JoinPath("orgs", orgLogin, "rulesets")
	if err != nil {
		return nil, err
	}
	return listRulesets(httpClient, u, limit, includeParents, host)
}

func listRulesets(httpClient *http.Client, u *safeurl.MutableSafeURL, limit int, includeParents bool, host string) (*RulesetList, error) {
	perPage := min(limit, 100)
	if perPage < 1 {
		perPage = 30
	}
	u.SetQuery("includes_parents", strconv.FormatBool(includeParents))
	u.SetQuery("per_page", strconv.Itoa(perPage))

	res := RulesetList{
		Rulesets: []RulesetGraphQL{},
	}
	client := api.NewClientFromHTTP(httpClient)

	nextPath := u.String()
	for nextPath != "" {
		var page []RulesetREST
		next, err := client.RESTWithNext(host, http.MethodGet, nextPath, nil, &page)
		if err != nil {
			if strings.Contains(err.Error(), "requires one of the following scopes: ['admin:org']") {
				return nil, errors.New("the 'admin:org' scope is required to view organization rulesets, try running 'gh auth refresh -s admin:org'")
			}

			return nil, err
		}

		for _, r := range page {
			gqlRS := RulesetGraphQL{
				DatabaseId:  r.Id,
				Name:        r.Name,
				Target:      strings.ToUpper(r.Target),
				Enforcement: strings.ToUpper(r.Enforcement),
			}
			gqlRS.Source.TypeName = r.SourceType
			gqlRS.Source.Owner = r.Source
			gqlRS.Rules.TotalCount = len(r.Rules)
			res.Rulesets = append(res.Rulesets, gqlRS)
			if limit > 0 && len(res.Rulesets) >= limit {
				res.TotalCount = len(res.Rulesets)
				return &res, nil
			}
		}

		nextPath = next
	}

	res.TotalCount = len(res.Rulesets)
	return &res, nil
}
