package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

// repoCreateInput is input parameters for the repoCreate method
type repoCreateInput struct {
	Name                 string
	HomepageURL          string
	Description          string
	Visibility           string
	OwnerLogin           string
	TeamSlug             string
	TemplateRepository   ghrepo.Interface
	TemplateRepositoryID string
	HasIssuesEnabled     bool
	HasWikiEnabled       bool
	GitIgnoreTemplate    string
	LicenseTemplate      string
	IncludeAllBranches   bool
	InitReadme           bool
}

// createRepositoryInputV3 is the payload for the repo create REST API
type createRepositoryInputV3 struct {
	Name              string `json:"name"`
	HomepageURL       string `json:"homepage,omitempty"`
	Description       string `json:"description,omitempty"`
	IsPrivate         bool   `json:"private"`
	Visibility        string `json:"visibility,omitempty"`
	TeamID            uint64 `json:"team_id,omitempty"`
	HasIssuesEnabled  bool   `json:"has_issues"`
	HasWikiEnabled    bool   `json:"has_wiki"`
	GitIgnoreTemplate string `json:"gitignore_template,omitempty"`
	LicenseTemplate   string `json:"license_template,omitempty"`
	InitReadme        bool   `json:"auto_init,omitempty"`
}

// repoCreate creates a new GitHub repository via REST.
func repoCreate(client *http.Client, hostname string, input repoCreateInput) (*api.Repository, error) {
	isOrg := false
	var teamIDv3 uint64

	apiClient := api.NewClientFromHTTP(client)

	if input.TeamSlug != "" {
		team, err := resolveOrganizationTeam(apiClient, hostname, input.OwnerLogin, input.TeamSlug)
		if err != nil {
			return nil, err
		}
		teamIDv3 = team.ID
		isOrg = true
	} else if input.OwnerLogin != "" {
		owner, err := resolveOwner(apiClient, hostname, input.OwnerLogin)
		if err != nil {
			return nil, err
		}
		isOrg = owner.IsOrganization()
	}

	isInternal := strings.ToLower(input.Visibility) == "internal"
	if isInternal && !isOrg {
		return nil, fmt.Errorf("internal repositories can only be created within an organization")
	}

	if input.TemplateRepository != nil {
		if isInternal {
			return nil, api.NotSupportedError("creating an internal repository from a template is not supported with REST")
		}

		targetOwner := input.OwnerLogin
		if targetOwner == "" {
			var err error
			targetOwner, err = api.CurrentLoginName(apiClient, hostname)
			if err != nil {
				return nil, err
			}
		}

		genPath, err := safeurl.JoinPath("repos", input.TemplateRepository.RepoOwner(), input.TemplateRepository.RepoName(), "generate")
		if err != nil {
			return nil, err
		}

		genBody := map[string]any{
			"owner":                targetOwner,
			"name":                 input.Name,
			"private":              strings.EqualFold(input.Visibility, "PRIVATE"),
			"include_all_branches": input.IncludeAllBranches,
		}
		if input.Description != "" {
			genBody["description"] = input.Description
		}
		payload, err := json.Marshal(genBody)
		if err != nil {
			return nil, err
		}

		repo, err := api.CreateRepoTransformToV4(apiClient, hostname, http.MethodPost, genPath, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}

		if !input.HasWikiEnabled || !input.HasIssuesEnabled || input.HomepageURL != "" {
			patchPath, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
			if err != nil {
				return nil, err
			}
			patchBody := map[string]any{
				"has_wiki":   input.HasWikiEnabled,
				"has_issues": input.HasIssuesEnabled,
			}
			if input.HomepageURL != "" {
				patchBody["homepage"] = input.HomepageURL
			}
			patchPayload, err := json.Marshal(patchBody)
			if err != nil {
				return nil, err
			}
			if err := apiClient.REST(hostname, http.MethodPatch, patchPath.String(), bytes.NewReader(patchPayload), nil); err != nil {
				return nil, err
			}
		}

		return repo, nil
	}

	inputv3 := createRepositoryInputV3{
		Name:              input.Name,
		HomepageURL:       input.HomepageURL,
		Description:       input.Description,
		IsPrivate:         strings.EqualFold(input.Visibility, "PRIVATE"),
		TeamID:            teamIDv3,
		HasIssuesEnabled:  input.HasIssuesEnabled,
		HasWikiEnabled:    input.HasWikiEnabled,
		GitIgnoreTemplate: input.GitIgnoreTemplate,
		LicenseTemplate:   input.LicenseTemplate,
		InitReadme:        input.InitReadme,
	}

	path, err := safeurl.JoinPath("user", "repos")
	if err != nil {
		return nil, err
	}
	if isOrg {
		path, err = safeurl.JoinPath("orgs", input.OwnerLogin, "repos")
		if err != nil {
			return nil, err
		}
		if input.Visibility != "" {
			inputv3.Visibility = strings.ToLower(input.Visibility)
		}
	}

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(inputv3); err != nil {
		return nil, err
	}

	return api.CreateRepoTransformToV4(apiClient, hostname, http.MethodPost, path, body)
}

type ownerResponse struct {
	NodeID string `json:"node_id"`
	Type   string `json:"type"`
}

func (r *ownerResponse) IsOrganization() bool {
	return r.Type == "Organization"
}

func resolveOwner(client *api.Client, hostname, orgName string) (*ownerResponse, error) {
	var response ownerResponse
	u, err := safeurl.JoinPath("users", orgName)
	if err != nil {
		return nil, err
	}
	err = client.REST(hostname, "GET", u.String(), nil, &response)
	return &response, err
}

type teamResponse struct {
	ID           uint64 `json:"id"`
	NodeID       string `json:"node_id"`
	Organization struct {
		NodeID string `json:"node_id"`
	}
}

func resolveOrganizationTeam(client *api.Client, hostname, orgName, teamSlug string) (*teamResponse, error) {
	var response teamResponse
	u, err := safeurl.JoinPath("orgs", orgName, "teams", teamSlug)
	if err != nil {
		return nil, err
	}
	err = client.REST(hostname, "GET", u.String(), nil, &response)
	return &response, err
}

func listTemplateRepositories(client *http.Client, hostname, owner string) ([]api.Repository, error) {
	apiClient := api.NewClientFromHTTP(client)
	_, repos, err := api.ListOwnerRepositoriesREST(apiClient, hostname, owner, 0, "", false, false)
	if err != nil {
		return nil, err
	}

	var templateRepositories []api.Repository
	for _, repo := range repos {
		if repo.IsTemplate {
			templateRepositories = append(templateRepositories, repo)
		}
	}

	return templateRepositories, nil
}

// Returns the current username and any orgs that user is a member of.
func userAndOrgs(httpClient *http.Client, hostname string) (string, []string, error) {
	client := api.NewClientFromHTTP(httpClient)
	return api.CurrentLoginNameAndOrgs(client, hostname)
}
