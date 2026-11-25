package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"golang.org/x/sync/errgroup"
)

const (
	errorResolvingOrganization = "Could not resolve to an Organization"
)

// Repository contains information about a GitHub repo
type Repository struct {
	ID                       string
	DatabaseID               int64
	Name                     string
	NameWithOwner            string
	Owner                    RepositoryOwner
	Parent                   *Repository
	TemplateRepository       *Repository
	Description              string
	HomepageURL              string
	OpenGraphImageURL        string
	UsesCustomOpenGraphImage bool
	URL                      string
	SSHURL                   string
	MirrorURL                string
	SecurityPolicyURL        string

	CreatedAt  time.Time
	PushedAt   *time.Time
	UpdatedAt  time.Time
	ArchivedAt *time.Time

	IsBlankIssuesEnabled    bool
	IsSecurityPolicyEnabled bool
	HasIssuesEnabled        bool
	HasProjectsEnabled      bool
	HasDiscussionsEnabled   bool
	HasWikiEnabled          bool
	MergeCommitAllowed      bool
	SquashMergeAllowed      bool
	RebaseMergeAllowed      bool
	AutoMergeAllowed        bool

	ForkCount      int
	StargazerCount int
	Watchers       struct {
		TotalCount int `json:"totalCount"`
	}
	Issues struct {
		TotalCount int `json:"totalCount"`
	}
	PullRequests struct {
		TotalCount int `json:"totalCount"`
	}

	CodeOfConduct                 *CodeOfConduct
	ContactLinks                  []ContactLink
	DefaultBranchRef              BranchRef
	DeleteBranchOnMerge           bool
	DiskUsage                     int
	FundingLinks                  []FundingLink
	IsArchived                    bool
	IsEmpty                       bool
	IsFork                        bool
	ForkingAllowed                bool
	IsInOrganization              bool
	IsMirror                      bool
	IsPrivate                     bool
	IsTemplate                    bool
	IsUserConfigurationRepository bool
	LicenseInfo                   *RepositoryLicense
	ViewerCanAdminister           bool
	ViewerDefaultCommitEmail      string
	ViewerDefaultMergeMethod      string
	ViewerHasStarred              bool
	ViewerPermission              string
	ViewerPossibleCommitEmails    []string
	ViewerSubscription            string
	Visibility                    string

	RepositoryTopics struct {
		Nodes []struct {
			Topic RepositoryTopic
		}
	}
	PrimaryLanguage *CodingLanguage
	Languages       struct {
		Edges []struct {
			Size int            `json:"size"`
			Node CodingLanguage `json:"node"`
		}
	}
	IssueTemplates       []IssueTemplate
	PullRequestTemplates []PullRequestTemplate
	Labels               struct {
		Nodes []IssueLabel
	}
	Milestones struct {
		Nodes []Milestone
	}
	LatestRelease *RepositoryRelease

	AssignableUsers struct {
		Nodes []GitHubUser
	}
	MentionableUsers struct {
		Nodes []GitHubUser
	}
	Projects struct {
		Nodes []RepoProject
	}
	ProjectsV2 struct {
		Nodes []ProjectV2
	}

	// pseudo-field that keeps track of host name of this repo
	hostname string
}

// RepositoryOwner is the owner of a GitHub repository
type RepositoryOwner struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

type GitHubUser struct {
	ID         string `json:"id"`
	Login      string `json:"login"`
	Name       string `json:"name"`
	DatabaseID int64  `json:"databaseId"`
}

// DisplayName returns a user-friendly name via actorDisplayName.
func (u GitHubUser) DisplayName() string {
	return actorDisplayName("", u.Login, u.Name)
}

// Actor is a superset of User and Bot, among others.
// At the time of writing, some of these fields
// are not directly supported by the Actor type and
// instead are only available on the User or Bot types
// directly.
type Actor struct {
	ID       string `json:"id"`
	Login    string `json:"login"`
	Name     string `json:"name"`
	TypeName string `json:"__typename"`
}

// BranchRef is the branch name in a GitHub repository
type BranchRef struct {
	Name string `json:"name"`
}

type CodeOfConduct struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type RepositoryLicense struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
}

type ContactLink struct {
	About string `json:"about"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

type FundingLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

type CodingLanguage struct {
	Name string `json:"name"`
}

type IssueTemplate struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Body  string `json:"body"`
	About string `json:"about"`
}

type PullRequestTemplate struct {
	Filename string `json:"filename"`
	Body     string `json:"body"`
}

type RepositoryTopic struct {
	Name string `json:"name"`
}

type RepositoryRelease struct {
	Name        string    `json:"name"`
	TagName     string    `json:"tagName"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"publishedAt"`
}

type IssueLabel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

type License struct {
	Key            string   `json:"key"`
	Name           string   `json:"name"`
	SPDXID         string   `json:"spdx_id"`
	URL            string   `json:"url"`
	NodeID         string   `json:"node_id"`
	HTMLURL        string   `json:"html_url"`
	Description    string   `json:"description"`
	Implementation string   `json:"implementation"`
	Permissions    []string `json:"permissions"`
	Conditions     []string `json:"conditions"`
	Limitations    []string `json:"limitations"`
	Body           string   `json:"body"`
	Featured       bool     `json:"featured"`
}

type GitIgnore struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// RepoOwner is the login name of the owner
func (r Repository) RepoOwner() string {
	return r.Owner.Login
}

// RepoName is the name of the repository
func (r Repository) RepoName() string {
	return r.Name
}

// RepoHost is the GitHub hostname of the repository
func (r Repository) RepoHost() string {
	return r.hostname
}

// ViewerCanPush is true when the requesting user has push access
func (r Repository) ViewerCanPush() bool {
	switch r.ViewerPermission {
	case "ADMIN", "MAINTAIN", "WRITE":
		return true
	default:
		return false
	}
}

// ViewerCanTriage is true when the requesting user can triage issues and pull requests
func (r Repository) ViewerCanTriage() bool {
	switch r.ViewerPermission {
	case "ADMIN", "MAINTAIN", "WRITE", "TRIAGE":
		return true
	default:
		return false
	}
}

// UnsupportedRepositoryJSONFields lists repository JSON fields that require
// GraphQL and cannot be populated from the single-repository REST endpoint.
var UnsupportedRepositoryJSONFields = []string{
	"assignableUsers",
	"codeOfConduct",
	"contactLinks",
	"fundingLinks",
	"isBlankIssuesEnabled",
	"isEmpty",
	"isSecurityPolicyEnabled",
	"isUserConfigurationRepository",
	"issueTemplates",
	"issues",
	"labels",
	"languages",
	"latestRelease",
	"mentionableUsers",
	"milestones",
	"projects",
	"projectsV2",
	"pullRequestTemplates",
	"pullRequests",
	"securityPolicyUrl",
	"viewerDefaultCommitEmail",
	"viewerDefaultMergeMethod",
	"viewerHasStarred",
	"viewerPossibleCommitEmails",
	"viewerSubscription",
}

// CheckUnsupportedRepositoryJSONFields returns a NotSupportedError if any
// explicitly requested repository JSON field is unavailable in REST mode.
func CheckUnsupportedRepositoryJSONFields(fields []string) error {
	for _, f := range fields {
		if slices.Contains(UnsupportedRepositoryJSONFields, f) {
			return NotSupportedError(fmt.Sprintf("the `%s` repository JSON field is not supported with REST", f))
		}
	}
	return nil
}

// FetchRepository fetches repository metadata via REST.
func FetchRepository(client *Client, repo ghrepo.Interface, fields []string) (*Repository, error) {
	res, err := fetchRESTRepo(client, repo)
	if err != nil {
		return nil, err
	}
	out := res.toRepository(repo.RepoHost())
	if out.Name == "" {
		out.Name = repo.RepoName()
	}
	if out.Owner.Login == "" {
		out.Owner.Login = repo.RepoOwner()
	}
	if out.NameWithOwner == "" && out.Owner.Login != "" && out.Name != "" {
		out.NameWithOwner = out.Owner.Login + "/" + out.Name
	}
	return out, nil
}

// ListOwnerRepositoriesREST lists repositories owned by the given owner (or the
// authenticated viewer when owner is empty) via REST.
func ListOwnerRepositoriesREST(client *Client, hostname, owner string, limit int, visibility string, forkOnly, sourceOnly bool) (string, []Repository, error) {
	perPage := 100
	if !forkOnly && !sourceOnly && limit > 0 && limit < 100 {
		perPage = limit
	}

	var u *safeurl.MutableSafeURL
	var err error
	ownerLogin := owner

	if owner == "" {
		u, err = safeurl.JoinPath("user", "repos")
		if err != nil {
			return "", nil, err
		}
		u.SetQuery("affiliation", "owner")
		if visibility != "" {
			u.SetQuery("visibility", strings.ToLower(visibility))
		}
	} else {
		userPath, pathErr := safeurl.JoinPath("users", owner)
		if pathErr != nil {
			return "", nil, pathErr
		}
		var ownerUser restUser
		if err := client.REST(hostname, http.MethodGet, userPath.String(), nil, &ownerUser); err != nil {
			if isHTTPNotFound(err) {
				return "", nil, nil
			}
			return "", nil, err
		}
		if ownerUser.Login != "" {
			ownerLogin = ownerUser.Login
		}
		if strings.EqualFold(ownerUser.Type, "Organization") {
			u, err = safeurl.JoinPath("orgs", ownerLogin, "repos")
			if err != nil {
				return "", nil, err
			}
			u.SetQuery("type", "all")
		} else {
			u, err = safeurl.JoinPath("users", ownerLogin, "repos")
			if err != nil {
				return "", nil, err
			}
			u.SetQuery("type", "owner")
		}
	}

	u.SetQuery("sort", "pushed")
	u.SetQuery("direction", "desc")
	u.SetQuery("per_page", strconv.Itoa(perPage))

	var repos []Repository
	nextPath := u.String()
	for nextPath != "" {
		var page []restRepo
		next, err := client.RESTWithNext(hostname, http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return "", nil, err
		}
		for i := range page {
			r := page[i].toRepository(hostname)
			if owner == "" && ownerLogin == "" && r.Owner.Login != "" {
				ownerLogin = r.Owner.Login
			}
			if visibility != "" && !strings.EqualFold(r.Visibility, visibility) {
				continue
			}
			if forkOnly && !r.IsFork {
				continue
			}
			if sourceOnly && r.IsFork {
				continue
			}
			repos = append(repos, *r)
			if limit > 0 && len(repos) >= limit {
				return ownerLogin, repos, nil
			}
		}
		nextPath = next
	}

	return ownerLogin, repos, nil
}

// SearchRepositoriesREST searches repositories using the REST search endpoint.
func SearchRepositoriesREST(client *Client, hostname, query string, limit int) (int, []Repository, error) {
	perPage := 100
	if limit > 0 && limit < 100 {
		perPage = limit
	}

	basePath, err := safeurl.JoinPath("search", "repositories")
	if err != nil {
		return 0, nil, err
	}

	var totalCount int
	var repos []Repository
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("q", query)
		q.Set("per_page", strconv.Itoa(perPage))
		q.Set("page", strconv.Itoa(page))
		reqPath := basePath.String() + "?" + q.Encode()

		var res struct {
			TotalCount int        `json:"total_count"`
			Items      []restRepo `json:"items"`
		}
		if err := client.REST(hostname, http.MethodGet, reqPath, nil, &res); err != nil {
			return 0, nil, err
		}
		totalCount = res.TotalCount
		for i := range res.Items {
			repos = append(repos, *res.Items[i].toRepository(hostname))
			if limit > 0 && len(repos) >= limit {
				return totalCount, repos, nil
			}
		}
		if len(res.Items) < perPage || len(res.Items) == 0 {
			break
		}
	}

	return totalCount, repos, nil
}

// IssueRepoInfo fetches repository metadata needed for issue operations via REST.
func IssueRepoInfo(client *Client, repo ghrepo.Interface) (*Repository, error) {
	res, err := fetchRESTRepo(client, repo)
	if err != nil {
		return nil, err
	}
	return res.toRepository(repo.RepoHost()), nil
}

// GitHubRepo fetches repository metadata via REST.
func GitHubRepo(client *Client, repo ghrepo.Interface) (*Repository, error) {
	res, err := fetchRESTRepo(client, repo)
	if err != nil {
		return nil, err
	}
	return res.toRepository(repo.RepoHost()), nil
}

func RepoDefaultBranch(client *Client, repo ghrepo.Interface) (string, error) {
	if r, ok := repo.(*Repository); ok && r.DefaultBranchRef.Name != "" {
		return r.DefaultBranchRef.Name, nil
	}

	r, err := GitHubRepo(client, repo)
	if err != nil {
		return "", err
	}
	return r.DefaultBranchRef.Name, nil
}

func CanPushToRepo(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	if r, ok := repo.(*Repository); ok && r.ViewerPermission != "" {
		return r.ViewerCanPush(), nil
	}

	apiClient := NewClientFromHTTP(httpClient)
	r, err := GitHubRepo(apiClient, repo)
	if err != nil {
		return false, err
	}
	return r.ViewerCanPush(), nil
}

// RepoParent finds out the parent repository of a fork via REST.
func RepoParent(client *Client, repo ghrepo.Interface) (ghrepo.Interface, error) {
	res, err := fetchRESTRepo(client, repo)
	if err != nil {
		return nil, err
	}
	if res.Parent == nil || res.Parent.Owner == nil {
		return nil, nil
	}

	return ghrepo.NewWithHost(res.Parent.Owner.Login, res.Parent.Name, repo.RepoHost()), nil
}

// RepoNetworkResult describes the relationship between related repositories
type RepoNetworkResult struct {
	ViewerLogin  string
	Repositories []*Repository
}

// RepoNetwork inspects the relationship between multiple GitHub repositories via REST.
func RepoNetwork(client *Client, repos []ghrepo.Interface) (RepoNetworkResult, error) {
	if len(repos) == 0 {
		return RepoNetworkResult{}, nil
	}

	hostname := repos[0].RepoHost()
	result := RepoNetworkResult{
		Repositories: make([]*Repository, len(repos)),
	}

	g, _ := errgroup.WithContext(context.Background())
	g.Go(func() error {
		login, err := CurrentLoginName(client, hostname)
		if err != nil {
			return err
		}
		result.ViewerLogin = login
		return nil
	})

	for i, repo := range repos {
		g.Go(func() error {
			res, err := fetchRESTRepo(client, repo)
			if err != nil {
				if isHTTPNotFound(err) {
					return nil
				}
				return err
			}
			result.Repositories[i] = res.toRepository(repo.RepoHost())
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return RepoNetworkResult{}, err
	}
	return result, nil
}

func InitRepoHostname(repo *Repository, hostname string) *Repository {
	repo.hostname = hostname
	if repo.Parent != nil {
		repo.Parent.hostname = hostname
	}
	return repo
}

// RepositoryV3 is the repository result from GitHub API v3
type repositoryV3 struct {
	NodeID    string `json:"node_id"`
	Name      string
	CreatedAt time.Time `json:"created_at"`
	Owner     struct {
		Login string
	}
	Private bool
	HTMLUrl string `json:"html_url"`
	Parent  *repositoryV3
}

// ForkRepo forks the repository on GitHub and returns the new repository
func ForkRepo(client *Client, repo ghrepo.Interface, org, newName string, defaultBranchOnly bool) (*Repository, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "forks")
	if err != nil {
		return nil, err
	}

	params := map[string]any{}
	if org != "" {
		params["organization"] = org
	}
	if newName != "" {
		params["name"] = newName
	}
	if defaultBranchOnly {
		params["default_branch_only"] = true
	}

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(params); err != nil {
		return nil, err
	}

	result := repositoryV3{}
	err = client.REST(repo.RepoHost(), "POST", path.String(), body, &result)
	if err != nil {
		return nil, err
	}

	newRepo := &Repository{
		ID:        result.NodeID,
		Name:      result.Name,
		CreatedAt: result.CreatedAt,
		Owner: RepositoryOwner{
			Login: result.Owner.Login,
		},
		ViewerPermission: "WRITE",
		hostname:         repo.RepoHost(),
	}

	// The GitHub API will happily return a HTTP 200 when attempting to fork own repo even though no forking
	// actually took place. Ensure that we raise an error instead.
	if ghrepo.IsSame(repo, newRepo) {
		return newRepo, fmt.Errorf("%s cannot be forked. A single user account cannot own both a parent and fork.", ghrepo.FullName(repo))
	}

	return newRepo, nil
}

// RenameRepo renames the repository on GitHub and returns the renamed repository
func RenameRepo(client *Client, repo ghrepo.Interface, newRepoName string) (*Repository, error) {
	input := map[string]string{"name": newRepoName}
	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(input); err != nil {
		return nil, err
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return nil, err
	}

	result := repositoryV3{}
	err = client.REST(repo.RepoHost(), http.MethodPatch, path.String(), body, &result)
	if err != nil {
		return nil, err
	}

	return &Repository{
		ID:        result.NodeID,
		Name:      result.Name,
		CreatedAt: result.CreatedAt,
		Owner: RepositoryOwner{
			Login: result.Owner.Login,
		},
		ViewerPermission: "WRITE",
		hostname:         repo.RepoHost(),
	}, nil
}

// LastCommit returns the latest commit OID on the repository's default branch via REST.
func LastCommit(client *Client, repo ghrepo.Interface) (*Commit, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "commits")
	if err != nil {
		return nil, err
	}
	path.SetQuery("per_page", "1")

	var commits []struct {
		SHA string `json:"sha"`
	}
	if err := client.REST(repo.RepoHost(), http.MethodGet, path.String(), nil, &commits); err != nil {
		if httpErr, ok := errors.AsType[HTTPError](err); ok && httpErr.StatusCode == http.StatusConflict {
			return &Commit{}, nil
		}
		return nil, err
	}
	if len(commits) == 0 {
		return &Commit{}, nil
	}
	return &Commit{OID: commits[0].SHA}, nil
}

// RepoFindForks finds pushable forks of the repo via REST.
// Because the REST forks endpoint does not filter by viewer affiliation, it first
// checks the viewer's own repository of the same name and then inspects recent forks.
func RepoFindForks(client *Client, repo ghrepo.Interface, limit int) ([]*Repository, error) {
	if limit <= 0 {
		return nil, nil
	}

	var results []*Repository
	seen := map[string]struct{}{}
	addFork := func(r *Repository) {
		if r == nil || !r.ViewerCanPush() {
			return
		}
		key := strings.ToLower(ghrepo.FullName(r))
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		results = append(results, r)
	}

	if viewerLogin, err := CurrentLoginName(client, repo.RepoHost()); err == nil && viewerLogin != "" && !strings.EqualFold(viewerLogin, repo.RepoOwner()) {
		ownCandidate := ghrepo.NewWithHost(viewerLogin, repo.RepoName(), repo.RepoHost())
		if ownRes, err := fetchRESTRepo(client, ownCandidate); err == nil {
			if ownRes.Fork && ownRes.Parent != nil && strings.EqualFold(ownRes.Parent.FullName, ghrepo.FullName(repo)) {
				addFork(ownRes.toRepository(repo.RepoHost()))
			}
		} else if !isHTTPNotFound(err) {
			return nil, err
		}
	}

	if len(results) >= limit {
		return results[:limit], nil
	}

	perPage := min(max(limit*5, 30), 100)
	forksPath, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "forks")
	if err != nil {
		return nil, err
	}
	forksPath.SetQuery("sort", "newest")
	forksPath.SetQuery("per_page", fmt.Sprintf("%d", perPage))

	var forks []restRepo
	if err := client.REST(repo.RepoHost(), http.MethodGet, forksPath.String(), nil, &forks); err != nil {
		return nil, err
	}

	for _, f := range forks {
		addFork(f.toRepository(repo.RepoHost()))
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

type RepoMetadataResult struct {
	CurrentLogin     string
	AssignableUsers  []AssignableUser
	AssignableActors []AssignableActor
	Labels           []RepoLabel
	Projects         []RepoProject
	ProjectsV2       []ProjectV2
	Milestones       []RepoMilestone
	Teams            []OrgTeam
}

func (m *RepoMetadataResult) MembersToIDs(names []string) ([]string, error) {
	var ids []string
	for _, assigneeLogin := range names {
		found := false
		for _, u := range m.AssignableUsers {
			if strings.EqualFold(assigneeLogin, u.Login()) {
				ids = append(ids, u.ID())
				found = true
				break
			}
		}

		// Look for ID in assignable actors if not found in assignable users
		if !found {
			for _, a := range m.AssignableActors {
				if strings.EqualFold(assigneeLogin, a.Login()) {
					ids = append(ids, a.ID())
					found = true
					break
				}
				if strings.EqualFold(assigneeLogin, a.DisplayName()) {
					ids = append(ids, a.ID())
					found = true
					break
				}
			}
		}

		// And if we still didn't find an ID, return an error
		if !found {
			return nil, fmt.Errorf("'%s' not found", assigneeLogin)
		}
	}
	return ids, nil
}

func (m *RepoMetadataResult) TeamsToIDs(names []string) ([]string, error) {
	var ids []string
	for _, teamSlug := range names {
		found := false
		slug := teamSlug[strings.IndexRune(teamSlug, '/')+1:]
		for _, t := range m.Teams {
			if strings.EqualFold(slug, t.Slug) {
				ids = append(ids, t.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("'%s' not found", teamSlug)
		}
	}
	return ids, nil
}

func (m *RepoMetadataResult) LabelsToIDs(names []string) ([]string, error) {
	var ids []string
	for _, labelName := range names {
		found := false
		for _, l := range m.Labels {
			if strings.EqualFold(labelName, l.Name) {
				ids = append(ids, l.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("'%s' not found", labelName)
		}
	}
	return ids, nil
}

// LabelsToNames validates label names case-insensitively against repository
// metadata and returns their canonical repository names.
func (m *RepoMetadataResult) LabelsToNames(names []string) ([]string, error) {
	var canonical []string
	for _, labelName := range names {
		found := false
		for _, l := range m.Labels {
			if strings.EqualFold(labelName, l.Name) {
				canonical = append(canonical, l.Name)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("'%s' not found", labelName)
		}
	}
	return canonical, nil
}

// ProjectsTitlesToIDs returns two arrays:
// - the first contains IDs of projects V1
// - the second contains IDs of projects V2
// - if neither project V1 or project V2 can be found with a given name, then an error is returned
func (m *RepoMetadataResult) ProjectsTitlesToIDs(titles []string) ([]string, []string, error) {
	var ids []string
	var idsV2 []string
	for _, title := range titles {
		id, found := m.v1ProjectNameToID(title)
		if found {
			ids = append(ids, id)
			continue
		}

		idV2, found := m.v2ProjectTitleToID(title)
		if found {
			idsV2 = append(idsV2, idV2)
			continue
		}

		return nil, nil, fmt.Errorf("'%s' not found", title)
	}
	return ids, idsV2, nil
}

// We use the word "titles" when referring to v1 and v2 projects.
// In reality, v1 projects really have "names", so there is a bit of a
// mismatch we just need to gloss over.
func (m *RepoMetadataResult) v1ProjectNameToID(name string) (string, bool) {
	for _, p := range m.Projects {
		if strings.EqualFold(name, p.Name) {
			return p.ID, true
		}
	}

	return "", false
}

func (m *RepoMetadataResult) v2ProjectTitleToID(title string) (string, bool) {
	for _, p := range m.ProjectsV2 {
		if strings.EqualFold(title, p.Title) {
			return p.ID, true
		}
	}

	return "", false
}

func ProjectTitlesToPaths(client *Client, repo ghrepo.Interface, titles []string, projectsV1Support gh.ProjectsV1Support) ([]string, error) {
	paths := make([]string, 0, len(titles))
	matchedPaths := map[string]struct{}{}

	// TODO: ProjectsV1Cleanup
	// At this point, we only know the names that the user has provided, so we can't push this conditional up the stack.
	// First we'll try to match against v1 projects, if supported
	if projectsV1Support == gh.ProjectsV1Supported {
		v1Projects, err := v1Projects(client, repo)
		if err != nil {
			return nil, err
		}

		for _, title := range titles {
			for _, p := range v1Projects {
				if strings.EqualFold(title, p.Name) {
					pathParts := strings.Split(p.ResourcePath, "/")
					var path string
					if pathParts[1] == "orgs" || pathParts[1] == "users" {
						path = fmt.Sprintf("%s/%s", pathParts[2], pathParts[4])
					} else {
						path = fmt.Sprintf("%s/%s/%s", pathParts[1], pathParts[2], pathParts[4])
					}
					paths = append(paths, path)
					matchedPaths[title] = struct{}{}
					break
				}
			}
		}
	}

	// Then we'll try to match against v2 projects
	v2Projects, err := v2Projects(client, repo)
	if err != nil {
		return nil, err
	}

	for _, title := range titles {
		// If we already found a v1 project with this name, skip it
		if _, ok := matchedPaths[title]; ok {
			continue
		}

		found := false
		for _, p := range v2Projects {
			if strings.EqualFold(title, p.Title) {
				pathParts := strings.Split(p.ResourcePath, "/")
				var path string
				if pathParts[1] == "orgs" || pathParts[1] == "users" {
					path = fmt.Sprintf("%s/%s", pathParts[2], pathParts[4])
				} else {
					path = fmt.Sprintf("%s/%s/%s", pathParts[1], pathParts[2], pathParts[4])
				}
				paths = append(paths, path)
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("'%s' not found", title)
		}
	}

	return paths, nil
}

func (m *RepoMetadataResult) MilestoneToID(title string) (string, error) {
	for _, m := range m.Milestones {
		if strings.EqualFold(title, m.Title) {
			return m.ID, nil
		}
	}
	return "", fmt.Errorf("'%s' not found", title)
}

// MilestoneToNumber resolves a milestone title (case-insensitively) to its
// numeric REST milestone number.
func (m *RepoMetadataResult) MilestoneToNumber(title string) (int, error) {
	for _, ms := range m.Milestones {
		if strings.EqualFold(title, ms.Title) {
			return ms.Number, nil
		}
	}
	return 0, fmt.Errorf("'%s' not found", title)
}

func (m *RepoMetadataResult) Merge(m2 *RepoMetadataResult) {
	if len(m2.AssignableUsers) > 0 || len(m.AssignableUsers) == 0 {
		m.AssignableUsers = m2.AssignableUsers
	}

	if len(m2.Teams) > 0 || len(m.Teams) == 0 {
		m.Teams = m2.Teams
	}

	if len(m2.Labels) > 0 || len(m.Labels) == 0 {
		m.Labels = m2.Labels
	}

	if len(m2.Projects) > 0 || len(m.Projects) == 0 {
		m.Projects = m2.Projects
	}

	if len(m2.Milestones) > 0 || len(m.Milestones) == 0 {
		m.Milestones = m2.Milestones
	}
}

type RepoMetadataInput struct {
	Assignees     bool
	Reviewers     bool
	TeamReviewers bool
	// TODO ApiActorsSupported
	ApiActorsSupported bool
	Labels             bool
	ProjectsV1         bool
	ProjectsV2         bool
	Milestones         bool
}

// RepoMetadata pre-fetches the metadata for attaching to issues and pull requests
func RepoMetadata(client *Client, repo ghrepo.Interface, input RepoMetadataInput) (*RepoMetadataResult, error) {
	var result RepoMetadataResult
	var g errgroup.Group

	if input.Assignees || input.Reviewers {
		g.Go(func() error {
			users, err := RepoAssignableUsers(client, repo)
			if err != nil {
				err = fmt.Errorf("error fetching assignable users: %w", err)
			}
			result.AssignableUsers = users
			return err
		})
	}

	if input.Reviewers && input.TeamReviewers {
		g.Go(func() error {
			teams, err := OrganizationTeams(client, repo)
			if err != nil && !isHTTPNotFoundOrNotOrg(err) {
				err = fmt.Errorf("error fetching organization teams: %w", err)
				return err
			}
			result.Teams = teams
			return nil
		})
	}

	if input.Reviewers {
		g.Go(func() error {
			login, err := CurrentLoginName(client, repo.RepoHost())
			if err != nil {
				err = fmt.Errorf("error fetching current login: %w", err)
			}
			result.CurrentLogin = login
			return err
		})
	}

	if input.Labels {
		g.Go(func() error {
			labels, err := RepoLabels(client, repo)
			if err != nil {
				err = fmt.Errorf("error fetching labels: %w", err)
			}
			result.Labels = labels
			return err
		})
	}

	if input.ProjectsV1 {
		g.Go(func() error {
			var err error
			result.Projects, err = v1Projects(client, repo)
			return err
		})
	}

	if input.ProjectsV2 {
		g.Go(func() error {
			var err error
			result.ProjectsV2, err = v2Projects(client, repo)
			return err
		})
	}

	if input.Milestones {
		g.Go(func() error {
			milestones, err := RepoMilestones(client, repo, "open")
			if err != nil {
				err = fmt.Errorf("error fetching milestones: %w", err)
			}
			result.Milestones = milestones
			return err
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &result, nil
}

type RepoProject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Number       int    `json:"number"`
	ResourcePath string `json:"resourcePath"`
}

// RepoProjects fetches all open classic projects for a repository via REST.
func RepoProjects(client *Client, repo ghrepo.Interface) ([]RepoProject, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "projects")
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

// Expected login for Copilot when retrieved as an assignee
const CopilotAssigneeLogin = "copilot-swe-agent"

// Expected login for Copilot when retrieved as a Pull Request Reviewer.
const CopilotReviewerLogin = "copilot-pull-request-reviewer"
const CopilotActorName = "Copilot"

// actorDisplayName returns a user-friendly display name for any actor.
// It handles bots (e.g. Copilot → "Copilot (AI)"), users with names
// ("login (Name)"), and falls back to just login. Empty typeName is
// treated as a possible bot or user — the login is checked against
// known bot logins first.
func actorDisplayName(typeName, login, name string) string {
	if login == CopilotReviewerLogin || login == CopilotAssigneeLogin || login == CopilotActorName {
		return fmt.Sprintf("%s (AI)", CopilotActorName)
	}
	if typeName == botTypeName {
		return login
	}
	if name != "" {
		return fmt.Sprintf("%s (%s)", login, name)
	}
	return login
}

type AssignableActor interface {
	DisplayName() string
	ID() string
	Login() string

	sealedAssignableActor()
}

// Always a user
type AssignableUser struct {
	id    string
	login string
	name  string
}

func NewAssignableUser(id, login, name string) AssignableUser {
	return AssignableUser{
		id:    id,
		login: login,
		name:  name,
	}
}

// DisplayName returns a user-friendly name via actorDisplayName.
func (u AssignableUser) DisplayName() string {
	return actorDisplayName(userTypeName, u.login, u.name)
}

func (u AssignableUser) ID() string {
	return u.id
}

func (u AssignableUser) Login() string {
	return u.login
}

func (u AssignableUser) Name() string {
	return u.name
}

func (u AssignableUser) sealedAssignableActor() {}

type AssignableBot struct {
	id    string
	login string
}

func NewAssignableBot(id, login string) AssignableBot {
	return AssignableBot{
		id:    id,
		login: login,
	}
}

func (b AssignableBot) DisplayName() string {
	return actorDisplayName(botTypeName, b.login, "")
}

func (b AssignableBot) ID() string {
	return b.id
}

func (b AssignableBot) Login() string {
	return b.login
}

func (b AssignableBot) Name() string {
	return ""
}

func (b AssignableBot) sealedAssignableActor() {}

// RepoAssignableUsers fetches all the assignable users for a repository via REST.
func RepoAssignableUsers(client *Client, repo ghrepo.Interface) ([]AssignableUser, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "assignees")
	if err != nil {
		return nil, err
	}
	path.SetQuery("per_page", "100")

	rawUsers, err := restPaginate[restUser](client, repo.RepoHost(), path.String(), 0)
	if err != nil {
		return nil, err
	}

	users := make([]AssignableUser, 0, len(rawUsers))
	for _, u := range rawUsers {
		users = append(users, AssignableUser{
			id:    u.NodeID,
			login: u.Login,
			name:  u.Name,
		})
	}
	return users, nil
}

type RepoLabel struct {
	ID   string
	Name string
}

// RepoLabels fetches all the labels in a repository via REST.
func RepoLabels(client *Client, repo ghrepo.Interface) ([]RepoLabel, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "labels")
	if err != nil {
		return nil, err
	}
	path.SetQuery("per_page", "100")

	rawLabels, err := restPaginate[restLabel](client, repo.RepoHost(), path.String(), 0)
	if err != nil {
		return nil, err
	}

	labels := make([]RepoLabel, 0, len(rawLabels))
	for _, l := range rawLabels {
		labels = append(labels, RepoLabel{
			ID:   l.NodeID,
			Name: l.Name,
		})
	}
	slices.SortFunc(labels, func(a, b RepoLabel) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return labels, nil
}

type RepoMilestone struct {
	ID     string
	Title  string
	Number int
}

// RepoMilestones fetches milestones in a repository via REST.
func RepoMilestones(client *Client, repo ghrepo.Interface, state string) ([]RepoMilestone, error) {
	switch state {
	case "open", "closed", "all":
	default:
		return nil, fmt.Errorf("invalid state: %s", state)
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "milestones")
	if err != nil {
		return nil, err
	}
	path.SetQuery("state", state)
	path.SetQuery("per_page", "100")

	rawMilestones, err := restPaginate[restMilestone](client, repo.RepoHost(), path.String(), 0)
	if err != nil {
		return nil, err
	}

	milestones := make([]RepoMilestone, 0, len(rawMilestones))
	for _, m := range rawMilestones {
		milestones = append(milestones, RepoMilestone{
			ID:     m.NodeID,
			Title:  m.Title,
			Number: m.Number,
		})
	}
	return milestones, nil
}

// v1Projects retrieves set of RepoProjects relevant to given repository:
// - Projects for repository
// - Projects for repository organization, if it belongs to one
func v1Projects(client *Client, repo ghrepo.Interface) ([]RepoProject, error) {
	var repoProjects []RepoProject
	var orgProjects []RepoProject

	g, _ := errgroup.WithContext(context.Background())

	g.Go(func() error {
		var err error
		repoProjects, err = RepoProjects(client, repo)
		if err != nil {
			err = fmt.Errorf("error fetching repo projects (classic): %w", err)
		}
		return err
	})

	g.Go(func() error {
		var err error
		orgProjects, err = OrganizationProjects(client, repo)
		if err != nil && !isHTTPNotFoundOrNotOrg(err) {
			err = fmt.Errorf("error fetching organization projects (classic): %w", err)
			return err
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	projects := make([]RepoProject, 0, len(repoProjects)+len(orgProjects))
	projects = append(projects, repoProjects...)
	projects = append(projects, orgProjects...)

	return projects, nil
}

// v2Projects retrieves set of ProjectV2 relevant to given repository:
// - ProjectsV2 owned by current user
// - ProjectsV2 linked to repository
// - ProjectsV2 owned by repository organization, if it belongs to one
func v2Projects(client *Client, repo ghrepo.Interface) ([]ProjectV2, error) {
	var userProjectsV2 []ProjectV2
	var repoProjectsV2 []ProjectV2
	var orgProjectsV2 []ProjectV2

	g, _ := errgroup.WithContext(context.Background())

	g.Go(func() error {
		var err error
		userProjectsV2, err = CurrentUserProjectsV2(client, repo.RepoHost())
		if err != nil && !ProjectsV2IgnorableError(err) {
			err = fmt.Errorf("error fetching user projects: %w", err)
			return err
		}
		return nil
	})

	g.Go(func() error {
		var err error
		repoProjectsV2, err = RepoProjectsV2(client, repo)
		if err != nil && !ProjectsV2IgnorableError(err) {
			err = fmt.Errorf("error fetching repo projects: %w", err)
			return err
		}
		return nil
	})

	g.Go(func() error {
		var err error
		orgProjectsV2, err = OrganizationProjectsV2(client, repo)
		if err != nil &&
			!ProjectsV2IgnorableError(err) &&
			!strings.Contains(err.Error(), errorResolvingOrganization) {
			err = fmt.Errorf("error fetching organization projects: %w", err)
			return err
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// ProjectV2 might appear across multiple queries so use a map to keep them deduplicated.
	m := make(map[string]ProjectV2, len(userProjectsV2)+len(repoProjectsV2)+len(orgProjectsV2))
	for _, p := range userProjectsV2 {
		m[p.ID] = p
	}
	for _, p := range repoProjectsV2 {
		m[p.ID] = p
	}
	for _, p := range orgProjectsV2 {
		m[p.ID] = p
	}
	projectsV2 := make([]ProjectV2, 0, len(m))
	for _, p := range m {
		projectsV2 = append(projectsV2, p)
	}

	return projectsV2, nil
}

func CreateRepoTransformToV4(apiClient *Client, hostname string, method string, path safeurl.SafeURL, body io.Reader) (*Repository, error) {
	var responsev3 repositoryV3
	err := apiClient.REST(hostname, method, path.String(), body, &responsev3)

	if err != nil {
		return nil, err
	}

	return &Repository{
		Name:      responsev3.Name,
		CreatedAt: responsev3.CreatedAt,
		Owner: RepositoryOwner{
			Login: responsev3.Owner.Login,
		},
		ID:        responsev3.NodeID,
		hostname:  hostname,
		URL:       responsev3.HTMLUrl,
		IsPrivate: responsev3.Private,
	}, nil
}

// GetRepoIDs retrieves a set of database IDs for the given set of repositories via REST.
func GetRepoIDs(client *Client, host string, repositories []ghrepo.Interface) ([]int64, error) {
	result := make([]int64, len(repositories))
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(10)

	for i, repo := range repositories {
		g.Go(func() error {
			target := ghrepo.NewWithHost(repo.RepoOwner(), repo.RepoName(), host)
			res, err := fetchRESTRepo(client, target)
			if err != nil {
				return fmt.Errorf("failed to look up repositories: %w", err)
			}
			result[i] = res.ID
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return result, nil
}

func RepoExists(client *Client, repo ghrepo.Interface) (bool, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return false, err
	}

	// A HEAD request has no body to decode, so Request is used rather than REST. Existence is
	// decided by the status alone.
	resp, err := client.Request(repo.RepoHost(), http.MethodHead, path.String(), nil)
	if err != nil {
		if httpErr, ok := errors.AsType[HTTPError](err); ok && httpErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}

	defer resp.Body.Close()

	// Only 200 means the repository exists. Any other success status is unexpected here and is
	// reported as an error rather than being taken as existence.
	if resp.StatusCode != http.StatusOK {
		return false, UnexpectedStatusError(resp)
	}

	return true, nil
}

// RepoLicenses fetches available repository licenses.
// It uses API v3 because licenses are not supported by GraphQL.
func RepoLicenses(httpClient *http.Client, hostname string) ([]License, error) {
	var licenses []License
	client := NewClientFromHTTP(httpClient)
	path, err := safeurl.JoinPath("licenses")
	if err != nil {
		return nil, err
	}
	err = client.REST(hostname, "GET", path.String(), nil, &licenses)
	if err != nil {
		return nil, err
	}
	return licenses, nil
}

// RepoLicense fetches an available repository license.
// It uses API v3 because licenses are not supported by GraphQL.
func RepoLicense(httpClient *http.Client, hostname string, licenseName string) (*License, error) {
	var license License
	client := NewClientFromHTTP(httpClient)
	path, err := safeurl.JoinPath("licenses", licenseName)
	if err != nil {
		return nil, err
	}
	err = client.REST(hostname, "GET", path.String(), nil, &license)
	if err != nil {
		return nil, err
	}
	return &license, nil
}

// RepoGitIgnoreTemplates fetches available repository gitignore templates.
// It uses API v3 here because gitignore template isn't supported by GraphQL.
func RepoGitIgnoreTemplates(httpClient *http.Client, hostname string) ([]string, error) {
	var gitIgnoreTemplates []string
	client := NewClientFromHTTP(httpClient)
	path, err := safeurl.JoinPath("gitignore", "templates")
	if err != nil {
		return nil, err
	}
	err = client.REST(hostname, "GET", path.String(), nil, &gitIgnoreTemplates)
	if err != nil {
		return nil, err
	}
	return gitIgnoreTemplates, nil
}

// RepoGitIgnoreTemplate fetches an available repository gitignore template.
// It uses API v3 here because gitignore template isn't supported by GraphQL.
func RepoGitIgnoreTemplate(httpClient *http.Client, hostname string, gitIgnoreTemplateName string) (*GitIgnore, error) {
	var gitIgnoreTemplate GitIgnore
	client := NewClientFromHTTP(httpClient)
	path, err := safeurl.JoinPath("gitignore", "templates", gitIgnoreTemplateName)
	if err != nil {
		return nil, err
	}
	err = client.REST(hostname, "GET", path.String(), nil, &gitIgnoreTemplate)
	if err != nil {
		return nil, err
	}
	return &gitIgnoreTemplate, nil
}
