package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"golang.org/x/sync/errgroup"
)

type IssuesPayload struct {
	Assigned  IssuesAndTotalCount
	Mentioned IssuesAndTotalCount
	Authored  IssuesAndTotalCount
}

type IssuesAndTotalCount struct {
	Issues       []Issue
	TotalCount   int
	SearchCapped bool
}

type Issue struct {
	Typename         string `json:"__typename"`
	ID               string
	DatabaseID       int64
	Number           int
	Title            string
	URL              string
	State            string
	StateReason      string
	Closed           bool
	Body             string
	ActiveLockReason string
	Locked           bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ClosedAt         *time.Time
	Comments         Comments
	Author           Author
	Assignees        Assignees
	AssignedActors   AssignedActors
	Labels           Labels
	ProjectCards     ProjectCards
	ProjectItems     ProjectItems
	Milestone        *Milestone
	ReactionGroups   ReactionGroups
	IsPinned         bool

	IssueType        *IssueType
	Parent           *LinkedIssue
	SubIssues        SubIssues
	SubIssuesSummary SubIssuesSummary
	BlockedBy        LinkedIssueConnection
	Blocking         LinkedIssueConnection

	ClosedByPullRequestsReferences ClosedByPullRequestsReferences

	// Repository is only populated when the "repository" field was requested.
	Repository *PRRepository
}

// IssueType represents an issue type configured for a repository.
type IssueType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// LinkedIssue represents a related issue (parent, sub-issue, or relationship target).
type LinkedIssue struct {
	ID         string `json:"id"`
	DatabaseID int64  `json:"databaseId,omitempty"`
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	State      string `json:"state"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

// SubIssues is a connection of sub-issues with a total count.
type SubIssues struct {
	Nodes      []LinkedIssue `json:"nodes"`
	TotalCount int           `json:"totalCount"`
}

// SubIssuesSummary contains completion stats for sub-issues.
type SubIssuesSummary struct {
	Total            int     `json:"total"`
	Completed        int     `json:"completed"`
	PercentCompleted float64 `json:"percentCompleted"`
}

// LinkedIssueConnection is a connection of related issues (blocked-by or blocking).
type LinkedIssueConnection struct {
	Nodes      []LinkedIssue `json:"nodes"`
	TotalCount int           `json:"totalCount"`
}

type ClosedByPullRequestsReferences struct {
	Nodes []struct {
		ID         string
		Number     int
		URL        string
		Repository struct {
			ID    string
			Name  string
			Owner struct {
				ID    string
				Login string
			}
		}
	}
	PageInfo struct {
		HasNextPage bool
		EndCursor   string
	}
}

// return values for Issue.Typename
const (
	TypeIssue       string = "Issue"
	TypePullRequest string = "PullRequest"
)

func (i Issue) IsPullRequest() bool {
	return i.Typename == TypePullRequest
}

type Assignees struct {
	Nodes      []GitHubUser
	TotalCount int
}

func (a Assignees) Logins() []string {
	logins := make([]string, len(a.Nodes))
	for i, a := range a.Nodes {
		logins[i] = a.Login
	}
	return logins
}

type AssignedActors struct {
	Nodes      []Actor
	TotalCount int
}

func (a AssignedActors) Logins() []string {
	logins := make([]string, len(a.Nodes))
	for i, a := range a.Nodes {
		logins[i] = a.Login
	}
	return logins
}

// DisplayNames returns a list of display names for the assigned actors.
func (a AssignedActors) DisplayNames() []string {
	// These display names are used for populating the "default" assigned actors
	// from the AssignedActors type. But, this is only one piece of the puzzle
	// as later, other queries will fetch the full list of possible assignable
	// actors from the repository, and the two lists will be reconciled.
	//
	// It's important that the display names are the same between the defaults
	// (the values returned here) and the full list (the values returned by
	// other repository queries). Any discrepancy would result in an
	// "invalid default", which means an assigned actor will not be matched
	// to an assignable actor and not presented as a "default" selection.
	// Not being presented as a default would cause the actor to be potentially
	// unassigned if the edits were submitted.
	//
	// To prevent this, we need shared logic to look up an actor's display name.
	// However, our API types between assignedActors and the full list of
	// assignableActors are different. So, as an attempt to maintain
	// consistency we convert the assignedActors to the same types as the
	// repository's assignableActors, treating the assignableActors DisplayName
	// methods as the sources of truth.
	// TODO KW: make this comment less of a wall of text if needed.
	var displayNames []string
	for _, a := range a.Nodes {
		if a.TypeName == "User" {
			u := NewAssignableUser(
				a.ID,
				a.Login,
				a.Name,
			)
			displayNames = append(displayNames, u.DisplayName())
		} else if a.TypeName == "Bot" {
			b := NewAssignableBot(
				a.ID,
				a.Login,
			)
			displayNames = append(displayNames, b.DisplayName())
		}
	}
	return displayNames
}

type Labels struct {
	Nodes      []IssueLabel
	TotalCount int
}

func (l Labels) Names() []string {
	names := make([]string, len(l.Nodes))
	for i, l := range l.Nodes {
		names[i] = l.Name
	}
	return names
}

type ProjectCards struct {
	Nodes      []*ProjectInfo
	TotalCount int
}

type ProjectItems struct {
	Nodes      []*ProjectV2Item
	TotalCount int
}

type ProjectInfo struct {
	Project struct {
		Name string `json:"name"`
	} `json:"project"`
	Column struct {
		Name string `json:"name"`
	} `json:"column"`
}

type ProjectV2Item struct {
	ID      string `json:"id"`
	Project ProjectV2ItemProject
	Status  ProjectV2ItemStatus
}

type ProjectV2ItemProject struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ProjectV2ItemStatus struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
}

func (p ProjectCards) ProjectNames() []string {
	names := make([]string, len(p.Nodes))
	for i, c := range p.Nodes {
		names[i] = c.Project.Name
	}
	return names
}

func (p ProjectItems) ProjectTitles() []string {
	titles := make([]string, len(p.Nodes))
	for i, c := range p.Nodes {
		titles[i] = c.Project.Title
	}
	return titles
}

type Milestone struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueOn       *time.Time `json:"dueOn"`
}

type IssuesDisabledError struct {
	error
}

type Owner struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Login string `json:"login"`
}

type Author struct {
	ID    string
	Name  string
	Login string
}

// DisplayName returns a user-friendly name via actorDisplayName.
func (a Author) DisplayName() string {
	return actorDisplayName("", a.Login, a.Name)
}

func (author Author) MarshalJSON() ([]byte, error) {
	if author.ID == "" {
		return json.Marshal(map[string]any{
			"is_bot": true,
			"login":  "app/" + author.Login,
		})
	}
	return json.Marshal(map[string]any{
		"is_bot": false,
		"login":  author.Login,
		"id":     author.ID,
		"name":   author.Name,
	})
}

type CommentAuthor struct {
	Login string `json:"login"`
	// Unfortunately, there is no easy way to add "id" and "name" fields to this struct because it's being
	// used in both shurcool-graphql type queries and string-based queries where the response gets parsed
	// by an ordinary JSON decoder that doesn't understand "graphql" directives via struct tags.
	//	User  *struct {
	//		ID   string
	//		Name string
	//	} `graphql:"... on User"`
}

// DisplayName returns a user-friendly name via actorDisplayName.
func (a CommentAuthor) DisplayName() string {
	return actorDisplayName("", a.Login, "")
}

// IssueCreate creates an issue in a GitHub repository via REST.
func IssueCreate(client *Client, repo ghrepo.Interface, params map[string]any) (*Issue, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues")
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{}
	for key, val := range params {
		switch key {
		case "title", "body":
			reqBody[key] = val
		case "assigneeLogins":
			if logins, ok := val.([]string); ok && len(logins) > 0 {
				reqBody["assignees"] = logins
			}
		case "labelNames":
			if names, ok := val.([]string); ok && len(names) > 0 {
				reqBody["labels"] = names
			}
		case "milestoneNumber":
			if num, ok := val.(int); ok && num > 0 {
				reqBody["milestone"] = num
			}
		case "type":
			if t, ok := val.(string); ok && t != "" {
				reqBody["type"] = t
			}
		case "issueTemplate":
			// REST POST /repos/{owner}/{repo}/issues does not accept issueTemplate.
		default:
			return nil, fmt.Errorf("invalid IssueCreate parameter %s", key)
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	var res restIssue
	if err := client.REST(repo.RepoHost(), http.MethodPost, path.String(), bytes.NewReader(bodyBytes), &res); err != nil {
		return nil, err
	}
	return res.toIssue(), nil
}

type IssueStatusOptions struct {
	Username string
	Fields   []string
}

func IssueStatus(client *Client, repo ghrepo.Interface, options IssueStatusOptions) (*IssuesPayload, error) {
	const perPage = 10
	fullName := ghrepo.FullName(repo)

	r, err := fetchRESTRepo(client, repo)
	if err != nil {
		return nil, err
	}
	if !r.HasIssues {
		return nil, fmt.Errorf("the '%s' repository has disabled issues", fullName)
	}

	var (
		assigned  *IssuesAndTotalCount
		mentioned *IssuesAndTotalCount
		authored  *IssuesAndTotalCount
	)

	g, _ := errgroup.WithContext(context.Background())

	g.Go(func() error {
		q := fmt.Sprintf("repo:%s is:issue is:open assignee:%s", fullName, options.Username)
		res, err := SearchIssuesREST(client, repo, q, "updated", perPage)
		if err != nil {
			return err
		}
		assigned = res
		return nil
	})

	g.Go(func() error {
		q := fmt.Sprintf("repo:%s is:issue is:open mentions:%s", fullName, options.Username)
		res, err := SearchIssuesREST(client, repo, q, "updated", perPage)
		if err != nil {
			return err
		}
		mentioned = res
		return nil
	})

	g.Go(func() error {
		q := fmt.Sprintf("repo:%s is:issue is:open author:%s", fullName, options.Username)
		res, err := SearchIssuesREST(client, repo, q, "updated", perPage)
		if err != nil {
			return err
		}
		authored = res
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &IssuesPayload{
		Assigned:  *assigned,
		Mentioned: *mentioned,
		Authored:  *authored,
	}, nil
}

func (i Issue) Link() string {
	return i.URL
}

func (i Issue) Identifier() string {
	return i.ID
}

func (i Issue) CurrentUserComments() []Comment {
	return i.Comments.CurrentUserComments()
}

// RepositoryDatabaseID returns the numeric REST id of the repository, or zero
// when the "repository" field was not requested.
func (i Issue) RepositoryDatabaseID() int64 {
	if i.Repository == nil {
		return 0
	}
	return i.Repository.DatabaseID
}

// RepositoryViewerPermission returns the requesting user's role on the
// repository, or an empty string when the "repository" field was not requested.
func (i Issue) RepositoryViewerPermission() string {
	if i.Repository == nil {
		return ""
	}
	return i.Repository.ViewerPermission
}

// IssueRef identifies an issue for REST relationship endpoints.
type IssueRef struct {
	Repo       ghrepo.Interface
	Number     int
	DatabaseID int64
	NodeID     string
}

// UpdateIssueIssueType sets or clears the issue type on an issue via REST. Pass
// an empty issueTypeName to clear the issue type.
func UpdateIssueIssueType(client *Client, repo ghrepo.Interface, number int, issueTypeName string) error {
	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(number))
	if err != nil {
		return err
	}
	var typeVal any
	if issueTypeName != "" {
		typeVal = issueTypeName
	}
	bodyBytes, err := json.Marshal(map[string]any{
		"type": typeVal,
	})
	if err != nil {
		return err
	}
	return client.REST(repo.RepoHost(), http.MethodPatch, p.String(), bytes.NewReader(bodyBytes), nil)
}

// AddSubIssue adds a sub-issue to a parent issue via REST.
func AddSubIssue(client *Client, repo ghrepo.Interface, parentNumber int, subIssueDatabaseID int64, replaceParent bool) error {
	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(parentNumber), "sub_issues")
	if err != nil {
		return err
	}
	bodyBytes, err := json.Marshal(map[string]any{
		"sub_issue_id":   subIssueDatabaseID,
		"replace_parent": replaceParent,
	})
	if err != nil {
		return err
	}
	return client.REST(repo.RepoHost(), http.MethodPost, p.String(), bytes.NewReader(bodyBytes), nil)
}

// RemoveSubIssue removes a sub-issue from a parent issue via REST.
func RemoveSubIssue(client *Client, repo ghrepo.Interface, parentNumber int, subIssueDatabaseID int64) error {
	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(parentNumber), "sub_issue")
	if err != nil {
		return err
	}
	bodyBytes, err := json.Marshal(map[string]any{
		"sub_issue_id": subIssueDatabaseID,
	})
	if err != nil {
		return err
	}
	return client.REST(repo.RepoHost(), http.MethodDelete, p.String(), bytes.NewReader(bodyBytes), nil)
}

// AddBlockedBy marks an issue as blocked by another issue via REST.
func AddBlockedBy(client *Client, repo ghrepo.Interface, issueNumber int, blockingDatabaseID int64) error {
	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(issueNumber), "dependencies", "blocked_by")
	if err != nil {
		return err
	}
	bodyBytes, err := json.Marshal(map[string]any{
		"issue_id": blockingDatabaseID,
	})
	if err != nil {
		return err
	}
	return client.REST(repo.RepoHost(), http.MethodPost, p.String(), bytes.NewReader(bodyBytes), nil)
}

// RemoveBlockedBy removes a "blocked by" relationship between two issues via REST.
func RemoveBlockedBy(client *Client, repo ghrepo.Interface, issueNumber int, blockingDatabaseID int64) error {
	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(issueNumber), "dependencies", "blocked_by", strconv.FormatInt(blockingDatabaseID, 10))
	if err != nil {
		return err
	}
	return client.REST(repo.RepoHost(), http.MethodDelete, p.String(), nil, nil)
}

// DeferredUpdateIssueOptions updates an issue with REST calls for issue type and
// parent/sub-issue/dependency relationships.
type DeferredUpdateIssueOptions struct {
	Repo            ghrepo.Interface
	IssueNumber     int
	IssueDatabaseID int64

	IssueTypeName   string
	RemoveIssueType bool

	Parent                *IssueRef
	ReplaceExistingParent bool
	RemoveParent          *IssueRef

	AddSubIssues    []IssueRef
	RemoveSubIssues []IssueRef

	AddBlockedBy    []IssueRef
	RemoveBlockedBy []IssueRef

	// AddBlocking / RemoveBlocking name issues that this issue blocks.
	// They are applied via the blocked_by endpoints with the arguments swapped.
	AddBlocking    []IssueRef
	RemoveBlocking []IssueRef
}

// DeferredUpdateIssue runs issue REST updates described by opts in parallel and
// returns any failures as a single joined error so a single failure does not
// abort the rest.
func DeferredUpdateIssue(client *Client, opts DeferredUpdateIssueOptions) error {
	var mutations []func() error

	if opts.IssueTypeName != "" || opts.RemoveIssueType {
		mutations = append(mutations, func() error {
			return UpdateIssueIssueType(client, opts.Repo, opts.IssueNumber, opts.IssueTypeName)
		})
	}

	if opts.Parent != nil {
		mutations = append(mutations, func() error {
			return AddSubIssue(client, opts.Parent.Repo, opts.Parent.Number, opts.IssueDatabaseID, opts.ReplaceExistingParent)
		})
	} else if opts.RemoveParent != nil {
		mutations = append(mutations, func() error {
			return RemoveSubIssue(client, opts.RemoveParent.Repo, opts.RemoveParent.Number, opts.IssueDatabaseID)
		})
	}

	for _, ref := range opts.AddSubIssues {
		mutations = append(mutations, func() error {
			return AddSubIssue(client, opts.Repo, opts.IssueNumber, ref.DatabaseID, true)
		})
	}
	for _, ref := range opts.RemoveSubIssues {
		mutations = append(mutations, func() error {
			return RemoveSubIssue(client, opts.Repo, opts.IssueNumber, ref.DatabaseID)
		})
	}

	for _, ref := range opts.AddBlockedBy {
		mutations = append(mutations, func() error {
			return AddBlockedBy(client, opts.Repo, opts.IssueNumber, ref.DatabaseID)
		})
	}
	for _, ref := range opts.RemoveBlockedBy {
		mutations = append(mutations, func() error {
			return RemoveBlockedBy(client, opts.Repo, opts.IssueNumber, ref.DatabaseID)
		})
	}

	for _, ref := range opts.AddBlocking {
		mutations = append(mutations, func() error {
			// Blocking is the inverse of blocked-by: this issue blocks ref.
			return AddBlockedBy(client, ref.Repo, ref.Number, opts.IssueDatabaseID)
		})
	}
	for _, ref := range opts.RemoveBlocking {
		mutations = append(mutations, func() error {
			return RemoveBlockedBy(client, ref.Repo, ref.Number, opts.IssueDatabaseID)
		})
	}

	if len(mutations) == 0 {
		return nil
	}

	errCh := make(chan error, len(mutations))
	var wg sync.WaitGroup
	for _, m := range mutations {
		wg.Add(1)
		go func(m func() error) {
			defer wg.Done()
			if err := m(); err != nil {
				errCh <- err
			}
		}(m)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// RepoIssueTypes fetches the available issue types for a repository's owner
// organization via REST. For user-owned repositories (404/403), it returns an
// empty slice.
func RepoIssueTypes(client *Client, repo ghrepo.Interface) ([]IssueType, error) {
	p, err := safeurl.JoinPath("orgs", repo.RepoOwner(), "issue-types")
	if err != nil {
		return nil, err
	}
	var raw []restIssueType
	if err := client.REST(repo.RepoHost(), http.MethodGet, p.String(), nil, &raw); err != nil {
		if httpErr, ok := err.(HTTPError); ok && (httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusForbidden) {
			return nil, nil
		}
		return nil, err
	}
	types := make([]IssueType, len(raw))
	for i := range raw {
		types[i] = *raw[i].toIssueType()
	}
	return types, nil
}

// IssueNodeID fetches the node ID for an issue given its number and repository.
func IssueNodeID(client *Client, repo ghrepo.Interface, number int) (string, error) {
	issue, err := SingleIssueREST(client, repo, number, nil)
	if err != nil {
		return "", err
	}
	return issue.ID, nil
}
