package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

// restPaginate fetches paginated JSON array items from a GitHub REST endpoint
// using Link headers until exhausted or maxItems is reached (if maxItems > 0).
func restPaginate[T any](client *Client, host, path string, maxItems int) ([]T, error) {
	var all []T
	nextPath := path
	for nextPath != "" {
		var page []T
		next, err := client.RESTWithNext(host, http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if maxItems > 0 && len(all) >= maxItems {
			all = all[:maxItems]
			break
		}
		nextPath = next
	}
	return all, nil
}

// isHTTPNotFound reports whether err is an HTTP 404 response.
func isHTTPNotFound(err error) bool {
	if httpErr, ok := errors.AsType[HTTPError](err); ok {
		return httpErr.StatusCode == http.StatusNotFound
	}
	return false
}

// isHTTPNotFoundOrNotOrg reports whether err is a 404 or legacy GraphQL org-resolution error.
func isHTTPNotFoundOrNotOrg(err error) bool {
	return isHTTPNotFound(err) || strings.Contains(err.Error(), errorResolvingOrganization)
}

type restUser struct {
	ID     int64  `json:"id"`
	NodeID string `json:"node_id"`
	Login  string `json:"login"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Email  string `json:"email"`
}

func (u *restUser) toAuthor() Author {
	if u == nil {
		return Author{}
	}
	if u.Type == "Bot" || strings.HasSuffix(u.Login, "[bot]") {
		// Author.MarshalJSON treats an empty ID as a GitHub App bot and prefixes "app/".
		return Author{
			Login: strings.TrimSuffix(u.Login, "[bot]"),
			Name:  u.Name,
		}
	}
	return Author{
		ID:    u.NodeID,
		Login: u.Login,
		Name:  u.Name,
	}
}

func (u *restUser) toGitHubUser() GitHubUser {
	if u == nil {
		return GitHubUser{}
	}
	return GitHubUser{
		ID:         u.NodeID,
		Login:      u.Login,
		Name:       u.Name,
		DatabaseID: u.ID,
	}
}

func (u *restUser) toOwner() Owner {
	if u == nil {
		return Owner{}
	}
	return Owner{
		ID:    u.NodeID,
		Name:  u.Name,
		Login: u.Login,
	}
}

type restPermissions struct {
	Admin    bool `json:"admin"`
	Maintain bool `json:"maintain"`
	Push     bool `json:"push"`
	Triage   bool `json:"triage"`
	Pull     bool `json:"pull"`
}

func (p *restPermissions) viewerPermission() string {
	if p == nil {
		return ""
	}
	switch {
	case p.Admin:
		return "ADMIN"
	case p.Maintain:
		return "MAINTAIN"
	case p.Push:
		return "WRITE"
	case p.Triage:
		return "TRIAGE"
	case p.Pull:
		return "READ"
	default:
		return ""
	}
}

type restLicense struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	SPDXID string `json:"spdx_id"`
}

type restRepo struct {
	ID                  int64            `json:"id"`
	NodeID              string           `json:"node_id"`
	Name                string           `json:"name"`
	FullName            string           `json:"full_name"`
	Owner               *restUser        `json:"owner"`
	Parent              *restRepo        `json:"parent"`
	TemplateRepository  *restRepo        `json:"template_repository"`
	Description         string           `json:"description"`
	Homepage            string           `json:"homepage"`
	HTMLURL             string           `json:"html_url"`
	SSHURL              string           `json:"ssh_url"`
	MirrorURL           string           `json:"mirror_url"`
	CreatedAt           time.Time        `json:"created_at"`
	PushedAt            *time.Time       `json:"pushed_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
	ArchivedAt          *time.Time       `json:"archived_at"`
	HasIssues           bool             `json:"has_issues"`
	HasProjects         bool             `json:"has_projects"`
	HasDiscussions      bool             `json:"has_discussions"`
	HasWiki             bool             `json:"has_wiki"`
	AllowMergeCommit    bool             `json:"allow_merge_commit"`
	AllowSquashMerge    bool             `json:"allow_squash_merge"`
	AllowRebaseMerge    bool             `json:"allow_rebase_merge"`
	AllowAutoMerge      bool             `json:"allow_auto_merge"`
	DeleteBranchOnMerge bool             `json:"delete_branch_on_merge"`
	ForksCount          int              `json:"forks_count"`
	StargazersCount     int              `json:"stargazers_count"`
	SubscribersCount    int              `json:"subscribers_count"`
	OpenIssuesCount     int              `json:"open_issues_count"`
	DefaultBranch       string           `json:"default_branch"`
	Size                int              `json:"size"`
	Archived            bool             `json:"archived"`
	Fork                bool             `json:"fork"`
	AllowForking        bool             `json:"allow_forking"`
	Private             bool             `json:"private"`
	IsTemplate          bool             `json:"is_template"`
	Visibility          string           `json:"visibility"`
	Language            string           `json:"language"`
	Topics              []string         `json:"topics"`
	License             *restLicense     `json:"license"`
	Permissions         *restPermissions `json:"permissions"`
}

func (r *restRepo) toRepository(hostname string) *Repository {
	if r == nil {
		return nil
	}

	var owner RepositoryOwner
	var isInOrg bool
	if r.Owner != nil {
		owner = RepositoryOwner{
			ID:    r.Owner.NodeID,
			Login: r.Owner.Login,
		}
		isInOrg = r.Owner.Type == "Organization"
	}

	visibility := strings.ToUpper(r.Visibility)
	if visibility == "" {
		if r.Private {
			visibility = "PRIVATE"
		} else {
			visibility = "PUBLIC"
		}
	}

	repo := &Repository{
		ID:                    r.NodeID,
		DatabaseID:            r.ID,
		Name:                  r.Name,
		NameWithOwner:         r.FullName,
		Owner:                 owner,
		Parent:                r.Parent.toRepository(hostname),
		TemplateRepository:    r.TemplateRepository.toRepository(hostname),
		Description:           r.Description,
		HomepageURL:           r.Homepage,
		URL:                   r.HTMLURL,
		SSHURL:                r.SSHURL,
		MirrorURL:             r.MirrorURL,
		CreatedAt:             r.CreatedAt,
		PushedAt:              r.PushedAt,
		UpdatedAt:             r.UpdatedAt,
		ArchivedAt:            r.ArchivedAt,
		HasIssuesEnabled:      r.HasIssues,
		HasProjectsEnabled:    r.HasProjects,
		HasDiscussionsEnabled: r.HasDiscussions,
		HasWikiEnabled:        r.HasWiki,
		MergeCommitAllowed:    r.AllowMergeCommit,
		SquashMergeAllowed:    r.AllowSquashMerge,
		RebaseMergeAllowed:    r.AllowRebaseMerge,
		AutoMergeAllowed:      r.AllowAutoMerge,
		DeleteBranchOnMerge:   r.DeleteBranchOnMerge,
		ForkCount:             r.ForksCount,
		StargazerCount:        r.StargazersCount,
		DefaultBranchRef:      BranchRef{Name: r.DefaultBranch},
		DiskUsage:             r.Size,
		IsArchived:            r.Archived,
		IsFork:                r.Fork,
		ForkingAllowed:        r.AllowForking,
		IsInOrganization:      isInOrg,
		IsMirror:              r.MirrorURL != "",
		IsPrivate:             r.Private,
		IsTemplate:            r.IsTemplate,
		ViewerCanAdminister:   r.Permissions != nil && r.Permissions.Admin,
		ViewerPermission:      r.Permissions.viewerPermission(),
		Visibility:            visibility,
		hostname:              hostname,
	}
	repo.Watchers.TotalCount = r.SubscribersCount

	if r.Language != "" {
		repo.PrimaryLanguage = &CodingLanguage{Name: r.Language}
	}
	if len(r.Topics) > 0 {
		repo.RepositoryTopics.Nodes = make([]struct {
			Topic RepositoryTopic
		}, len(r.Topics))
		for i, t := range r.Topics {
			repo.RepositoryTopics.Nodes[i].Topic = RepositoryTopic{Name: t}
		}
	}
	if r.License != nil && r.License.Key != "" {
		repo.LicenseInfo = &RepositoryLicense{
			Key:      r.License.Key,
			Name:     r.License.Name,
			Nickname: r.License.SPDXID,
		}
	}

	return repo
}

func (r *restRepo) toPRRepository() *PRRepository {
	if r == nil {
		return nil
	}
	return &PRRepository{
		ID:               r.NodeID,
		Name:             r.Name,
		NameWithOwner:    r.FullName,
		DatabaseID:       r.ID,
		ViewerPermission: r.Permissions.viewerPermission(),
	}
}

type restReactions struct {
	PlusOne  int `json:"+1"`
	MinusOne int `json:"-1"`
	Laugh    int `json:"laugh"`
	Hooray   int `json:"hooray"`
	Confused int `json:"confused"`
	Heart    int `json:"heart"`
	Rocket   int `json:"rocket"`
	Eyes     int `json:"eyes"`
}

func (rr *restReactions) toReactionGroups() ReactionGroups {
	if rr == nil {
		return nil
	}
	ordered := []struct {
		content string
		count   int
	}{
		{"THUMBS_UP", rr.PlusOne},
		{"THUMBS_DOWN", rr.MinusOne},
		{"LAUGH", rr.Laugh},
		{"HOORAY", rr.Hooray},
		{"CONFUSED", rr.Confused},
		{"HEART", rr.Heart},
		{"ROCKET", rr.Rocket},
		{"EYES", rr.Eyes},
	}
	groups := make(ReactionGroups, 0, len(ordered))
	for _, o := range ordered {
		if o.count > 0 {
			groups = append(groups, ReactionGroup{
				Content: o.content,
				Users:   ReactionGroupUsers{TotalCount: o.count},
			})
		}
	}
	return groups
}

type restLabel struct {
	ID          int64  `json:"id"`
	NodeID      string `json:"node_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Default     bool   `json:"default"`
	URL         string `json:"url"`
}

func (l restLabel) toIssueLabel() IssueLabel {
	return IssueLabel{
		ID:          l.NodeID,
		Name:        l.Name,
		Description: l.Description,
		Color:       l.Color,
	}
}

type restMilestone struct {
	Number      int        `json:"number"`
	NodeID      string     `json:"node_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	State       string     `json:"state"`
	DueOn       *time.Time `json:"due_on"`
}

func (m *restMilestone) toMilestone() *Milestone {
	if m == nil {
		return nil
	}
	return &Milestone{
		Number:      m.Number,
		Title:       m.Title,
		Description: m.Description,
		DueOn:       m.DueOn,
	}
}

type restProjectV1 struct {
	NodeID  string `json:"node_id"`
	Name    string `json:"name"`
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

func (p restProjectV1) toRepoProject() RepoProject {
	var resourcePath string
	if u, err := url.Parse(p.HTMLURL); err == nil {
		resourcePath = u.Path
	}
	return RepoProject{
		ID:           p.NodeID,
		Name:         p.Name,
		Number:       p.Number,
		ResourcePath: resourcePath,
	}
}

func fetchRESTRepo(client *Client, repo ghrepo.Interface) (*restRepo, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return nil, err
	}
	var res restRepo
	if err := client.REST(repo.RepoHost(), http.MethodGet, path.String(), nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
