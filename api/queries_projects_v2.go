package api

import (
	"errors"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
)

const (
	errorProjectsV2ReadScope             = "field requires one of the following scopes: ['read:project']"
	errorProjectsV2UserField             = "Field 'projectsV2' doesn't exist on type 'User'"
	errorProjectsV2RepositoryField       = "Field 'projectsV2' doesn't exist on type 'Repository'"
	errorProjectsV2OrganizationField     = "Field 'projectsV2' doesn't exist on type 'Organization'"
	errorProjectsV2IssueField            = "Field 'projectItems' doesn't exist on type 'Issue'"
	errorProjectsV2PullRequestField      = "Field 'projectItems' doesn't exist on type 'PullRequest'"
	errorProjectsV2ResourceNotAccessible = "Resource not accessible by"
)

type ProjectV2 struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Number       int    `json:"number"`
	ResourcePath string `json:"resourcePath"`
	Closed       bool   `json:"closed"`
	URL          string `json:"url"`
}

// UpdateProjectV2Items returns NotSupportedError because Projects V2 item
// mutations require GraphQL and are not available via REST.
func UpdateProjectV2Items(client *Client, repo ghrepo.Interface, addProjectItems, deleteProjectItems map[string]string) error {
	if len(addProjectItems)+len(deleteProjectItems) == 0 {
		return nil
	}
	return NotSupportedError("Projects V2 item updates are not supported with REST")
}

// ProjectsV2ItemsForIssue returns NotSupportedError because Projects V2 items
// on issues require GraphQL and are not available via REST.
func ProjectsV2ItemsForIssue(client *Client, repo ghrepo.Interface, issue *Issue) error {
	return NotSupportedError("Projects V2 items are not supported with REST")
}

// ProjectsV2ItemsForPullRequest returns NotSupportedError because Projects V2
// items on pull requests require GraphQL and are not available via REST.
func ProjectsV2ItemsForPullRequest(client *Client, repo ghrepo.Interface, pr *PullRequest) error {
	return NotSupportedError("Projects V2 items are not supported with REST")
}

// OrganizationProjectsV2 returns NotSupportedError because listing organization
// Projects V2 requires GraphQL and is not available via REST.
func OrganizationProjectsV2(client *Client, repo ghrepo.Interface) ([]ProjectV2, error) {
	return nil, NotSupportedError("organization Projects V2 are not supported with REST")
}

// RepoProjectsV2 returns NotSupportedError because listing repository
// Projects V2 requires GraphQL and is not available via REST.
func RepoProjectsV2(client *Client, repo ghrepo.Interface) ([]ProjectV2, error) {
	return nil, NotSupportedError("repository Projects V2 are not supported with REST")
}

// CurrentUserProjectsV2 returns NotSupportedError because listing user
// Projects V2 requires GraphQL and is not available via REST.
func CurrentUserProjectsV2(client *Client, hostname string) ([]ProjectV2, error) {
	return nil, NotSupportedError("user Projects V2 are not supported with REST")
}

// ProjectsV2IgnorableError reports whether a Projects V2 query error can be
// ignored during best-effort enrichment (missing scopes, unsupported fields, or
// REST-unsupported Projects V2 lookups).
func ProjectsV2IgnorableError(err error) bool {
	if errors.Is(err, ErrNotSupported) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, errorProjectsV2ReadScope) ||
		strings.Contains(msg, errorProjectsV2UserField) ||
		strings.Contains(msg, errorProjectsV2RepositoryField) ||
		strings.Contains(msg, errorProjectsV2OrganizationField) ||
		strings.Contains(msg, errorProjectsV2IssueField) ||
		strings.Contains(msg, errorProjectsV2PullRequestField) ||
		strings.Contains(msg, errorProjectsV2ResourceNotAccessible) {
		return true
	}
	return false
}
