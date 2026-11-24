package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"golang.org/x/sync/errgroup"
)

// UnsupportedIssueJSONFields lists issue JSON fields that require GraphQL and
// cannot be populated from GitHub REST API endpoints.
var UnsupportedIssueJSONFields = []string{
	"projectCards",
	"projectItems",
	"closedByPullRequestsReferences",
	"isPinned",
}

// CheckUnsupportedIssueJSONFields returns a NotSupportedError if any
// explicitly requested issue JSON field is unavailable in REST mode.
func CheckUnsupportedIssueJSONFields(fields []string) error {
	for _, f := range fields {
		if slices.Contains(UnsupportedIssueJSONFields, f) {
			return NotSupportedError(fmt.Sprintf("the `%s` issue JSON field is not supported with REST", f))
		}
	}
	return nil
}

type restIssueType struct {
	ID          int64  `json:"id"`
	NodeID      string `json:"node_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

func (t *restIssueType) toIssueType() *IssueType {
	if t == nil {
		return nil
	}
	id := t.NodeID
	if id == "" && t.ID != 0 {
		id = strconv.FormatInt(t.ID, 10)
	}
	return &IssueType{
		ID:          id,
		Name:        t.Name,
		Description: t.Description,
		Color:       t.Color,
	}
}

type restSubIssuesSummary struct {
	Total            int `json:"total"`
	Completed        int `json:"completed"`
	PercentCompleted int `json:"percent_completed"`
}

type restLinkedIssue struct {
	ID            int64  `json:"id"`
	NodeID        string `json:"node_id"`
	Number        int    `json:"number"`
	Title         string `json:"title"`
	HTMLURL       string `json:"html_url"`
	URL           string `json:"url"`
	State         string `json:"state"`
	RepositoryURL string `json:"repository_url"`
	Repository    *struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (r *restLinkedIssue) toLinkedIssue(fallbackRepo ghrepo.Interface) LinkedIssue {
	id := r.NodeID
	if id == "" && r.ID != 0 {
		id = strconv.FormatInt(r.ID, 10)
	}

	var nameWithOwner string
	if r.Repository != nil && r.Repository.FullName != "" {
		nameWithOwner = r.Repository.FullName
	} else if parsed := parseRepoFromIssueOrAPIURL(r.HTMLURL); parsed != "" {
		nameWithOwner = parsed
	} else if parsed := parseRepoFromAPIURL(r.RepositoryURL); parsed != "" {
		nameWithOwner = parsed
	} else if parsed := parseRepoFromAPIURL(r.URL); parsed != "" {
		nameWithOwner = parsed
	} else if fallbackRepo != nil {
		nameWithOwner = ghrepo.FullName(fallbackRepo)
	}

	li := LinkedIssue{
		ID:     id,
		Number: r.Number,
		Title:  r.Title,
		URL:    r.HTMLURL,
		State:  strings.ToUpper(r.State),
	}
	li.Repository.NameWithOwner = nameWithOwner
	return li
}

func parseRepoFromIssueOrAPIURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" && parts[0] != "repos" {
		return parts[0] + "/" + parts[1]
	}
	return ""
}

func parseRepoFromAPIURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "repos" && parts[i+1] != "" && parts[i+2] != "" {
			return parts[i+1] + "/" + parts[i+2]
		}
	}
	return ""
}

type restIssue struct {
	ID               int64                 `json:"id"`
	NodeID           string                `json:"node_id"`
	Number           int                   `json:"number"`
	Title            string                `json:"title"`
	State            string                `json:"state"`
	StateReason      string                `json:"state_reason"`
	Body             string                `json:"body"`
	HTMLURL          string                `json:"html_url"`
	Locked           bool                  `json:"locked"`
	ActiveLockReason string                `json:"active_lock_reason"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	ClosedAt         *time.Time            `json:"closed_at"`
	Comments         int                   `json:"comments"`
	User             *restUser             `json:"user"`
	Assignee         *restUser             `json:"assignee"`
	Assignees        []restUser            `json:"assignees"`
	Labels           []restLabel           `json:"labels"`
	Milestone        *restMilestone        `json:"milestone"`
	Reactions        *restReactions        `json:"reactions"`
	Type             *restIssueType        `json:"type"`
	SubIssuesSummary *restSubIssuesSummary `json:"sub_issues_summary"`
	PullRequest      *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
}

func (r *restIssue) toIssue() *Issue {
	typename := TypeIssue
	if r.PullRequest != nil {
		typename = TypePullRequest
	}
	state := strings.ToUpper(r.State)
	stateReason := strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(r.StateReason))
	lockReason := strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(r.ActiveLockReason))

	issue := &Issue{
		Typename:         typename,
		ID:               r.NodeID,
		DatabaseID:       r.ID,
		Number:           r.Number,
		Title:            r.Title,
		URL:              r.HTMLURL,
		State:            state,
		StateReason:      stateReason,
		Closed:           state == "CLOSED",
		Body:             r.Body,
		ActiveLockReason: lockReason,
		Locked:           r.Locked,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ClosedAt:         r.ClosedAt,
		Author:           r.User.toAuthor(),
		Milestone:        r.Milestone.toMilestone(),
		ReactionGroups:   r.Reactions.toReactionGroups(),
		IssueType:        r.Type.toIssueType(),
	}
	issue.Comments.TotalCount = r.Comments

	if r.SubIssuesSummary != nil {
		issue.SubIssuesSummary = SubIssuesSummary{
			Total:            r.SubIssuesSummary.Total,
			Completed:        r.SubIssuesSummary.Completed,
			PercentCompleted: float64(r.SubIssuesSummary.PercentCompleted),
		}
	}

	assignees := r.Assignees
	if len(assignees) == 0 && r.Assignee != nil {
		assignees = []restUser{*r.Assignee}
	}
	if len(assignees) > 0 {
		issue.Assignees.TotalCount = len(assignees)
		issue.AssignedActors.TotalCount = len(assignees)
		for i := range assignees {
			u := &assignees[i]
			issue.Assignees.Nodes = append(issue.Assignees.Nodes, u.toGitHubUser())
			typeName := userTypeName
			if u.Type == botTypeName || strings.HasSuffix(u.Login, "[bot]") {
				typeName = botTypeName
			}
			issue.AssignedActors.Nodes = append(issue.AssignedActors.Nodes, Actor{
				ID:       u.NodeID,
				Login:    u.Login,
				Name:     u.Name,
				TypeName: typeName,
			})
		}
	}

	if len(r.Labels) > 0 {
		issue.Labels.TotalCount = len(r.Labels)
		issue.Labels.Nodes = make([]IssueLabel, len(r.Labels))
		for i, l := range r.Labels {
			issue.Labels.Nodes[i] = l.toIssueLabel()
		}
	}

	return issue
}

func isHTTPGone(err error) bool {
	if httpErr, ok := err.(HTTPError); ok {
		return httpErr.StatusCode == http.StatusGone
	}
	return false
}

// SingleIssueREST fetches an issue or pull request by number via REST and
// hydrates requested secondary fields (comments, parent, subIssues, blockedBy,
// blocking, repository).
func SingleIssueREST(client *Client, repo ghrepo.Interface, number int, fields []string) (*Issue, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(number))
	if err != nil {
		return nil, err
	}

	var res restIssue
	if err := client.REST(repo.RepoHost(), http.MethodGet, path.String(), nil, &res); err != nil {
		if isHTTPGone(err) {
			return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
		}
		return nil, err
	}

	issue := res.toIssue()

	wants := make(map[string]bool, len(fields))
	for _, f := range fields {
		wants[f] = true
	}

	g, _ := errgroup.WithContext(context.Background())

	if wants["comments"] || (wants["lastComment"] && issue.Comments.TotalCount > 0) {
		g.Go(func() error {
			comments, err := FetchIssueCommentsREST(client, repo, number)
			if err != nil {
				return err
			}
			issue.Comments.Nodes = comments
			if len(comments) > issue.Comments.TotalCount {
				issue.Comments.TotalCount = len(comments)
			}
			return nil
		})
	}

	if wants["parent"] && !issue.IsPullRequest() {
		g.Go(func() error {
			p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(number), "parent")
			if err != nil {
				return err
			}
			var parent restLinkedIssue
			if err := client.REST(repo.RepoHost(), http.MethodGet, p.String(), nil, &parent); err != nil {
				if isHTTPNotFound(err) {
					return nil
				}
				return err
			}
			li := parent.toLinkedIssue(repo)
			issue.Parent = &li
			return nil
		})
	}

	if wants["subIssues"] && !issue.IsPullRequest() {
		g.Go(func() error {
			items, err := fetchLinkedIssueList(client, repo, number, "sub_issues")
			if err != nil {
				return err
			}
			issue.SubIssues = SubIssues{
				Nodes:      items,
				TotalCount: len(items),
			}
			return nil
		})
	}

	if wants["blockedBy"] && !issue.IsPullRequest() {
		g.Go(func() error {
			items, err := fetchLinkedIssueList(client, repo, number, "dependencies/blocked_by")
			if err != nil {
				return err
			}
			issue.BlockedBy = LinkedIssueConnection{
				Nodes:      items,
				TotalCount: len(items),
			}
			return nil
		})
	}

	if wants["blocking"] && !issue.IsPullRequest() {
		g.Go(func() error {
			items, err := fetchLinkedIssueList(client, repo, number, "dependencies/blocking")
			if err != nil {
				return err
			}
			issue.Blocking = LinkedIssueConnection{
				Nodes:      items,
				TotalCount: len(items),
			}
			return nil
		})
	}

	if wants["repository"] {
		g.Go(func() error {
			r, err := fetchRESTRepo(client, repo)
			if err != nil {
				return err
			}
			issue.Repository = r.toPRRepository()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return issue, nil
}

func fetchLinkedIssueList(client *Client, repo ghrepo.Interface, number int, suffix string) ([]LinkedIssue, error) {
	parts := []string{"repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(number)}
	parts = append(parts, strings.Split(suffix, "/")...)
	p, err := safeurl.JoinPath(parts...)
	if err != nil {
		return nil, err
	}
	p.SetQuery("per_page", "100")

	rawItems, err := restPaginate[restLinkedIssue](client, repo.RepoHost(), p.String(), 0)
	if err != nil {
		if isHTTPNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	items := make([]LinkedIssue, len(rawItems))
	for i := range rawItems {
		items[i] = rawItems[i].toLinkedIssue(repo)
	}
	return items, nil
}

// ListIssuesREST lists issues (excluding pull requests) for a repository via
// GET /repos/{owner}/{repo}/issues.
func ListIssuesREST(client *Client, repo ghrepo.Interface, state, assignee, author, mention string, labels []string, milestone string, limit int) (*IssuesAndTotalCount, error) {
	var restState string
	switch strings.ToLower(state) {
	case "open", "":
		restState = "open"
	case "closed":
		restState = "closed"
	case "all":
		restState = "all"
	default:
		return nil, fmt.Errorf("invalid state: %s", state)
	}

	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues")
	if err != nil {
		return nil, err
	}
	perPage := 100
	if limit > 0 && limit < 100 {
		// Still request 100 because GitHub's issues endpoint interleaves pull requests.
		perPage = 100
	}
	p.SetQuery("per_page", strconv.Itoa(perPage))
	p.SetQuery("state", restState)
	if assignee != "" {
		p.SetQuery("assignee", assignee)
	}
	if author != "" {
		p.SetQuery("creator", author)
	}
	if mention != "" {
		p.SetQuery("mentioned", mention)
	}
	if len(labels) > 0 {
		p.SetQuery("labels", strings.Join(labels, ","))
	}
	if milestone != "" {
		p.SetQuery("milestone", milestone)
	}

	res := &IssuesAndTotalCount{}
	seen := make(map[int]struct{})
	nextPath := p.String()

loop:
	for nextPath != "" {
		var page []restIssue
		next, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, nextPath, nil, &page)
		if err != nil {
			if isHTTPGone(err) {
				return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
			}
			return nil, err
		}

		for i := range page {
			if page[i].PullRequest != nil {
				continue
			}
			issue := page[i].toIssue()
			if issue.Number > 0 {
				if _, exists := seen[issue.Number]; exists {
					continue
				}
				seen[issue.Number] = struct{}{}
			}
			res.Issues = append(res.Issues, *issue)
			if limit > 0 && len(res.Issues) == limit {
				break loop
			}
		}
		nextPath = next
	}

	res.TotalCount = len(res.Issues)
	return res, nil
}

type restSearchIssuesResponse struct {
	TotalCount int         `json:"total_count"`
	Items      []restIssue `json:"items"`
}

// SearchIssuesREST searches issues via REST GET /search/issues.
func SearchIssuesREST(client *Client, repo ghrepo.Interface, query, sortField string, limit int) (*IssuesAndTotalCount, error) {
	p, err := safeurl.JoinPath("search", "issues")
	if err != nil {
		return nil, err
	}
	p.SetQuery("q", query)
	if sortField != "" {
		p.SetQuery("sort", sortField)
		p.SetQuery("order", "desc")
	}
	perPage := 100
	if limit > 0 && limit < 100 {
		perPage = limit
	}
	p.SetQuery("per_page", strconv.Itoa(perPage))

	res := &IssuesAndTotalCount{SearchCapped: limit > 1000}
	seen := make(map[int]struct{})
	nextPath := p.String()

loop:
	for nextPath != "" {
		var page restSearchIssuesResponse
		next, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, err
		}
		res.TotalCount = page.TotalCount

		for i := range page.Items {
			issue := page.Items[i].toIssue()
			if issue.Number > 0 {
				if _, exists := seen[issue.Number]; exists {
					continue
				}
				seen[issue.Number] = struct{}{}
			}
			res.Issues = append(res.Issues, *issue)
			if limit > 0 && len(res.Issues) == limit {
				break loop
			}
		}
		nextPath = next
	}

	return res, nil
}
