package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type restTeam struct {
	NodeID string `json:"node_id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
}

// OrganizationProjects fetches all open classic projects for an organization via REST.
func OrganizationProjects(client *Client, repo ghrepo.Interface) ([]RepoProject, error) {
	path, err := safeurl.JoinPath("orgs", repo.RepoOwner(), "projects")
	if err != nil {
		return nil, err
	}
	path.SetQuery("state", "open")
	path.SetQuery("per_page", "100")

	rawProjects, err := restPaginate[restProjectV1](client, repo.RepoHost(), path.String(), 0)
	if err != nil {
		return nil, err
	}

	projects := make([]RepoProject, 0, len(rawProjects))
	for _, p := range rawProjects {
		projects = append(projects, p.toRepoProject())
	}
	slices.SortFunc(projects, func(a, b RepoProject) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return projects, nil
}

type OrgTeam struct {
	ID   string
	Slug string
}

// OrganizationTeam fetches the team in an organization with the given slug via REST.
func OrganizationTeam(client *Client, hostname string, org string, teamSlug string) (*OrgTeam, error) {
	path, err := safeurl.JoinPath("orgs", org, "teams", teamSlug)
	if err != nil {
		return nil, err
	}

	var team restTeam
	if err := client.REST(hostname, http.MethodGet, path.String(), nil, &team); err != nil {
		return nil, err
	}

	return &OrgTeam{
		ID:   team.NodeID,
		Slug: team.Slug,
	}, nil
}

// OrganizationTeams fetches all the teams in an organization via REST.
func OrganizationTeams(client *Client, repo ghrepo.Interface) ([]OrgTeam, error) {
	path, err := safeurl.JoinPath("orgs", repo.RepoOwner(), "teams")
	if err != nil {
		return nil, err
	}
	path.SetQuery("per_page", "100")

	rawTeams, err := restPaginate[restTeam](client, repo.RepoHost(), path.String(), 0)
	if err != nil {
		return nil, err
	}

	teams := make([]OrgTeam, 0, len(rawTeams))
	for _, t := range rawTeams {
		teams = append(teams, OrgTeam{
			ID:   t.NodeID,
			Slug: t.Slug,
		})
	}
	slices.SortFunc(teams, func(a, b OrgTeam) int {
		return strings.Compare(strings.ToLower(a.Slug), strings.ToLower(b.Slug))
	})
	return teams, nil
}
