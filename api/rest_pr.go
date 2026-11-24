package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"golang.org/x/sync/errgroup"
)

type restPullBranch struct {
	Ref   string    `json:"ref"`
	SHA   string    `json:"sha"`
	Label string    `json:"label"`
	User  *restUser `json:"user"`
	Repo  *restRepo `json:"repo"`
}

type restRequestedTeam struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type restAutoMerge struct {
	EnabledBy     *restUser `json:"enabled_by"`
	MergeMethod   string    `json:"merge_method"`
	CommitTitle   *string   `json:"commit_title"`
	CommitMessage *string   `json:"commit_message"`
}

type restPullRequest struct {
	ID                  int64               `json:"id"`
	NodeID              string              `json:"node_id"`
	Number              int                 `json:"number"`
	Title               string              `json:"title"`
	State               string              `json:"state"`
	Body                string              `json:"body"`
	HTMLURL             string              `json:"html_url"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	ClosedAt            *time.Time          `json:"closed_at"`
	MergedAt            *time.Time          `json:"merged_at"`
	Merged              bool                `json:"merged"`
	Draft               bool                `json:"draft"`
	MaintainerCanModify bool                `json:"maintainer_can_modify"`
	Additions           int                 `json:"additions"`
	Deletions           int                 `json:"deletions"`
	ChangedFiles        int                 `json:"changed_files"`
	Commits             int                 `json:"commits"`
	Comments            int                 `json:"comments"`
	Mergeable           *bool               `json:"mergeable"`
	MergeableState      string              `json:"mergeable_state"`
	MergeCommitSHA      string              `json:"merge_commit_sha"`
	User                *restUser           `json:"user"`
	MergedBy            *restUser           `json:"merged_by"`
	Assignee            *restUser           `json:"assignee"`
	Assignees           []restUser          `json:"assignees"`
	Labels              []restLabel         `json:"labels"`
	Milestone           *restMilestone      `json:"milestone"`
	Head                *restPullBranch     `json:"head"`
	Base                *restPullBranch     `json:"base"`
	RequestedReviewers  []restUser          `json:"requested_reviewers"`
	RequestedTeams      []restRequestedTeam `json:"requested_teams"`
	AutoMerge           *restAutoMerge      `json:"auto_merge"`
}

func (r *restPullRequest) toPullRequest(repo ghrepo.Interface) *PullRequest {
	state := strings.ToUpper(r.State)
	closed := state == "CLOSED"
	if r.Merged || r.MergedAt != nil {
		state = "MERGED"
		closed = true
	}

	mergeable := PullRequestMergeableUnknown
	if r.Mergeable != nil {
		if *r.Mergeable {
			mergeable = PullRequestMergeableMergeable
		} else {
			mergeable = PullRequestMergeableConflicting
		}
	}

	pr := &PullRequest{
		ID:                  r.NodeID,
		Number:              r.Number,
		Title:               r.Title,
		State:               state,
		Closed:              closed,
		URL:                 r.HTMLURL,
		Body:                r.Body,
		Mergeable:           mergeable,
		Additions:           r.Additions,
		Deletions:           r.Deletions,
		ChangedFiles:        r.ChangedFiles,
		MergeStateStatus:    strings.ToUpper(r.MergeableState),
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
		ClosedAt:            r.ClosedAt,
		MergedAt:            r.MergedAt,
		Author:              r.User.toAuthor(),
		IsDraft:             r.Draft,
		MaintainerCanModify: r.MaintainerCanModify,
		Milestone:           r.Milestone.toMilestone(),
	}
	pr.Commits.TotalCount = r.Commits
	pr.Comments.TotalCount = r.Comments

	if r.ID != 0 {
		pr.FullDatabaseID = strconv.FormatInt(r.ID, 10)
	}
	if r.MergedBy != nil {
		mergedBy := r.MergedBy.toAuthor()
		pr.MergedBy = &mergedBy
	}
	if r.MergeCommitSHA != "" {
		pr.MergeCommit = &Commit{OID: r.MergeCommitSHA}
	}
	if r.AutoMerge != nil {
		pr.AutoMergeRequest = &AutoMergeRequest{
			CommitBody:     r.AutoMerge.CommitMessage,
			CommitHeadline: r.AutoMerge.CommitTitle,
			MergeMethod:    strings.ToUpper(r.AutoMerge.MergeMethod),
			EnabledBy:      r.AutoMerge.EnabledBy.toAuthor(),
		}
	}

	assignees := r.Assignees
	if len(assignees) == 0 && r.Assignee != nil {
		assignees = []restUser{*r.Assignee}
	}
	if len(assignees) > 0 {
		pr.Assignees.TotalCount = len(assignees)
		pr.AssignedActors.TotalCount = len(assignees)
		for i := range assignees {
			u := &assignees[i]
			pr.Assignees.Nodes = append(pr.Assignees.Nodes, u.toGitHubUser())
			typeName := userTypeName
			if u.Type == botTypeName || strings.HasSuffix(u.Login, "[bot]") {
				typeName = botTypeName
			}
			pr.AssignedActors.Nodes = append(pr.AssignedActors.Nodes, Actor{
				ID:       u.NodeID,
				Login:    u.Login,
				Name:     u.Name,
				TypeName: typeName,
			})
		}
	}

	if len(r.Labels) > 0 {
		pr.Labels.TotalCount = len(r.Labels)
		pr.Labels.Nodes = make([]IssueLabel, len(r.Labels))
		for i, l := range r.Labels {
			pr.Labels.Nodes[i] = l.toIssueLabel()
		}
	}

	if r.Head != nil {
		pr.HeadRefName = r.Head.Ref
		pr.HeadRefOid = r.Head.SHA
		if r.Head.User != nil {
			pr.HeadRepositoryOwner = r.Head.User.toOwner()
		} else if r.Head.Repo != nil && r.Head.Repo.Owner != nil {
			pr.HeadRepositoryOwner = r.Head.Repo.Owner.toOwner()
		}
		if r.Head.Repo != nil {
			pr.HeadRepository = r.Head.Repo.toPRRepository()
		}
	}
	if r.Base != nil {
		pr.BaseRefName = r.Base.Ref
		pr.BaseRefOid = r.Base.SHA
		if r.Base.Repo != nil {
			pr.Repository = r.Base.Repo.toPRRepository()
		}
	}

	if r.Head != nil && r.Base != nil {
		switch {
		case r.Head.Repo != nil && r.Base.Repo != nil:
			if r.Head.Repo.FullName != "" && r.Base.Repo.FullName != "" {
				pr.IsCrossRepository = r.Head.Repo.FullName != r.Base.Repo.FullName
			} else if r.Head.Repo.ID != 0 && r.Base.Repo.ID != 0 {
				pr.IsCrossRepository = r.Head.Repo.ID != r.Base.Repo.ID
			}
		case r.Head.Repo == nil && r.Head.User != nil && r.Base.Repo != nil && r.Base.Repo.Owner != nil:
			pr.IsCrossRepository = r.Head.User.Login != r.Base.Repo.Owner.Login
		case r.Head.User != nil && repo != nil && pr.HeadRepositoryOwner.Login != "":
			pr.IsCrossRepository = pr.HeadRepositoryOwner.Login != repo.RepoOwner()
		}
	}

	for _, u := range r.RequestedReviewers {
		typeName := userTypeName
		if u.Type == botTypeName || strings.HasSuffix(u.Login, "[bot]") {
			typeName = botTypeName
		}
		pr.ReviewRequests.Nodes = append(pr.ReviewRequests.Nodes, struct{ RequestedReviewer RequestedReviewer }{
			RequestedReviewer: RequestedReviewer{
				TypeName: typeName,
				Login:    u.Login,
				Name:     u.Name,
			},
		})
	}
	orgLogin := ""
	if repo != nil {
		orgLogin = repo.RepoOwner()
	}
	for _, t := range r.RequestedTeams {
		rr := RequestedReviewer{
			TypeName: teamTypeName,
			Name:     t.Name,
			Slug:     t.Slug,
		}
		rr.Organization.Login = orgLogin
		pr.ReviewRequests.Nodes = append(pr.ReviewRequests.Nodes, struct{ RequestedReviewer RequestedReviewer }{
			RequestedReviewer: rr,
		})
	}

	return pr
}

// FetchPullRequestREST fetches a single pull request by number via REST.
func FetchPullRequestREST(client *Client, repo ghrepo.Interface, number int) (*PullRequest, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls", strconv.Itoa(number))
	if err != nil {
		return nil, err
	}
	var res restPullRequest
	if err := client.REST(repo.RepoHost(), http.MethodGet, path.String(), nil, &res); err != nil {
		return nil, err
	}
	return res.toPullRequest(repo), nil
}

// FetchPullRequestsForBranchREST lists pull requests matching a qualified or unqualified head branch
// and state filters via REST, also returning the repository's default branch if present in base.repo.
func FetchPullRequestsForBranchREST(client *Client, repo ghrepo.Interface, headQuery string, stateFilters []string) ([]PullRequest, string, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls")
	if err != nil {
		return nil, "", err
	}
	path.SetQuery("per_page", "30")
	if headQuery != "" {
		path.SetQuery("head", headQuery)
	}

	restState := "all"
	if len(stateFilters) == 1 && stateFilters[0] == "OPEN" {
		restState = "open"
	} else if len(stateFilters) > 0 && !slices.Contains(stateFilters, "OPEN") {
		restState = "closed"
	}
	path.SetQuery("state", restState)

	var items []restPullRequest
	if err := client.REST(repo.RepoHost(), http.MethodGet, path.String(), nil, &items); err != nil {
		return nil, "", err
	}

	var defaultBranch string
	prs := make([]PullRequest, 0, len(items))
	for i := range items {
		if defaultBranch == "" && items[i].Base != nil && items[i].Base.Repo != nil {
			defaultBranch = items[i].Base.Repo.DefaultBranch
		}
		pr := items[i].toPullRequest(repo)
		if len(stateFilters) > 0 && !slices.Contains(stateFilters, pr.State) {
			continue
		}
		prs = append(prs, *pr)
	}
	return prs, defaultBranch, nil
}

type restPullRequestReview struct {
	ID                int64      `json:"id"`
	NodeID            string     `json:"node_id"`
	User              *restUser  `json:"user"`
	AuthorAssociation string     `json:"author_association"`
	Body              string     `json:"body"`
	SubmittedAt       *time.Time `json:"submitted_at"`
	State             string     `json:"state"`
	HTMLURL           string     `json:"html_url"`
	CommitID          string     `json:"commit_id"`
}

// FetchPRReviewsREST fetches all reviews for a pull request via REST.
func FetchPRReviewsREST(client *Client, repo ghrepo.Interface, number int) ([]PullRequestReview, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls", strconv.Itoa(number), "reviews")
	if err != nil {
		return nil, err
	}
	path.SetQuery("per_page", "100")

	items, err := restPaginate[restPullRequestReview](client, repo.RepoHost(), path.String(), 0)
	if err != nil {
		return nil, err
	}

	reviews := make([]PullRequestReview, 0, len(items))
	for _, r := range items {
		var author CommentAuthor
		if r.User != nil {
			author = CommentAuthor{Login: r.User.toAuthor().MarshalLogin()}
		}
		id := r.NodeID
		if id == "" && r.ID != 0 {
			id = strconv.FormatInt(r.ID, 10)
		}
		reviews = append(reviews, PullRequestReview{
			ID:                id,
			Author:            author,
			AuthorAssociation: r.AuthorAssociation,
			Body:              r.Body,
			SubmittedAt:       r.SubmittedAt,
			State:             strings.ToUpper(r.State),
			URL:               r.HTMLURL,
			Commit:            Commit{OID: r.CommitID},
		})
	}
	return reviews, nil
}

func (a Author) MarshalLogin() string {
	if a.ID == "" && a.Login != "" {
		return "app/" + a.Login
	}
	return a.Login
}

// ComputeLatestReviews derives the most recent non-pending review per author, matching
// GraphQL latestReviews semantics where plain COMMENTED reviews do not supersede
// APPROVED, CHANGES_REQUESTED, or DISMISSED reviews.
func ComputeLatestReviews(reviews []PullRequestReview) PullRequestReviews {
	latestIdx := make(map[string]int)
	var order []string
	for i, r := range reviews {
		if r.State == "PENDING" || r.Author.Login == "" {
			continue
		}
		login := strings.ToLower(r.Author.Login)
		prevIdx, exists := latestIdx[login]
		if !exists {
			latestIdx[login] = i
			order = append(order, login)
			continue
		}
		prevState := reviews[prevIdx].State
		if r.State == "COMMENTED" && (prevState == "APPROVED" || prevState == "CHANGES_REQUESTED" || prevState == "DISMISSED") {
			continue
		}
		latestIdx[login] = i
	}

	nodes := make([]PullRequestReview, 0, len(order))
	for _, login := range order {
		nodes = append(nodes, reviews[latestIdx[login]])
	}
	return PullRequestReviews{
		Nodes:      nodes,
		TotalCount: len(nodes),
	}
}

type restIssueComment struct {
	ID                int64          `json:"id"`
	NodeID            string         `json:"node_id"`
	User              *restUser      `json:"user"`
	AuthorAssociation string         `json:"author_association"`
	Body              string         `json:"body"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	HTMLURL           string         `json:"html_url"`
	Reactions         *restReactions `json:"reactions"`
}

// FetchIssueCommentsREST fetches all issue/PR timeline comments via REST.
func FetchIssueCommentsREST(client *Client, repo ghrepo.Interface, number int) ([]Comment, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(number), "comments")
	if err != nil {
		return nil, err
	}
	path.SetQuery("per_page", "100")

	items, err := restPaginate[restIssueComment](client, repo.RepoHost(), path.String(), 0)
	if err != nil {
		return nil, err
	}

	comments := make([]Comment, 0, len(items))
	for _, c := range items {
		var author CommentAuthor
		if c.User != nil {
			author = CommentAuthor{Login: c.User.toAuthor().MarshalLogin()}
		}
		id := c.NodeID
		if id == "" && c.ID != 0 {
			id = strconv.FormatInt(c.ID, 10)
		}
		comments = append(comments, Comment{
			ID:                  id,
			DatabaseID:          c.ID,
			Author:              author,
			AuthorAssociation:   c.AuthorAssociation,
			Body:                c.Body,
			CreatedAt:           c.CreatedAt,
			IncludesCreatedEdit: !c.UpdatedAt.IsZero() && c.UpdatedAt.After(c.CreatedAt),
			ReactionGroups:      c.Reactions.toReactionGroups(),
			URL:                 c.HTMLURL,
		})
	}
	return comments, nil
}

type restPullRequestFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// FetchPRFilesREST fetches all changed files for a pull request via REST.
func FetchPRFilesREST(client *Client, repo ghrepo.Interface, number int) ([]PullRequestFile, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls", strconv.Itoa(number), "files")
	if err != nil {
		return nil, err
	}
	path.SetQuery("per_page", "100")

	items, err := restPaginate[restPullRequestFile](client, repo.RepoHost(), path.String(), 0)
	if err != nil {
		return nil, err
	}

	files := make([]PullRequestFile, 0, len(items))
	for _, f := range items {
		changeType := strings.ToUpper(f.Status)
		if changeType == "REMOVED" {
			changeType = "DELETED"
		}
		files = append(files, PullRequestFile{
			Path:       f.Filename,
			Additions:  f.Additions,
			Deletions:  f.Deletions,
			ChangeType: changeType,
		})
	}
	return files, nil
}

type restPRCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name  string    `json:"name"`
			Email string    `json:"email"`
			Date  time.Time `json:"date"`
		} `json:"author"`
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
	Author *restUser `json:"author"`
}

// FetchPRCommitsREST fetches all commits on a pull request via REST.
func FetchPRCommitsREST(client *Client, repo ghrepo.Interface, number int) ([]PullRequestCommit, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls", strconv.Itoa(number), "commits")
	if err != nil {
		return nil, err
	}
	path.SetQuery("per_page", "100")

	items, err := restPaginate[restPRCommit](client, repo.RepoHost(), path.String(), 0)
	if err != nil {
		return nil, err
	}

	commits := make([]PullRequestCommit, 0, len(items))
	for _, c := range items {
		headline, body, _ := strings.Cut(strings.ReplaceAll(c.Commit.Message, "\r\n", "\n"), "\n")
		body = strings.TrimLeft(body, "\n")

		var authorUser GitHubUser
		if c.Author != nil {
			authorUser = c.Author.toGitHubUser()
		}

		var prCommit PullRequestCommit
		prCommit.Commit = PullRequestCommitCommit{
			OID:             c.SHA,
			MessageHeadline: headline,
			MessageBody:     body,
			AuthoredDate:    c.Commit.Author.Date,
			CommittedDate:   c.Commit.Committer.Date,
		}
		prCommit.Commit.Authors.Nodes = []struct {
			Name  string
			Email string
			User  GitHubUser
		}{
			{
				Name:  c.Commit.Author.Name,
				Email: c.Commit.Author.Email,
				User:  authorUser,
			},
		}
		commits = append(commits, prCommit)
	}
	return commits, nil
}

type restIssueReactionsOnly struct {
	Reactions *restReactions `json:"reactions"`
}

// FetchPRReactionsREST fetches reaction counts for a PR or issue via the REST issues endpoint.
func FetchPRReactionsREST(client *Client, repo ghrepo.Interface, number int) (ReactionGroups, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(number))
	if err != nil {
		return nil, err
	}
	var res restIssueReactionsOnly
	if err := client.REST(repo.RepoHost(), http.MethodGet, path.String(), nil, &res); err != nil {
		return nil, err
	}
	return res.Reactions.toReactionGroups(), nil
}

// HydratePullRequestFields fetches any requested subresource fields (reviews, latestReviews,
// comments, files, commits, statusCheckRollup, reactionGroups) via REST and populates pr.
func HydratePullRequestFields(client *Client, repo ghrepo.Interface, pr *PullRequest, fields []string) error {
	var wantReviews, wantComments, wantFiles, wantCommits, wantChecks, wantReactions bool
	for _, f := range fields {
		switch f {
		case "reviews", "latestReviews":
			wantReviews = true
		case "comments":
			wantComments = true
		case "files":
			wantFiles = true
		case "commits":
			wantCommits = true
		case "statusCheckRollup":
			wantChecks = true
		case "reactionGroups":
			wantReactions = true
		}
	}

	g, _ := errgroup.WithContext(context.Background())

	if wantReviews && len(pr.Reviews.Nodes) == 0 {
		g.Go(func() error {
			reviews, err := FetchPRReviewsREST(client, repo, pr.Number)
			if err != nil {
				return err
			}
			pr.Reviews = PullRequestReviews{
				Nodes:      reviews,
				TotalCount: len(reviews),
			}
			pr.LatestReviews = ComputeLatestReviews(reviews)
			return nil
		})
	}

	if wantComments && len(pr.Comments.Nodes) == 0 {
		g.Go(func() error {
			comments, err := FetchIssueCommentsREST(client, repo, pr.Number)
			if err != nil {
				return err
			}
			pr.Comments = Comments{
				Nodes:      comments,
				TotalCount: len(comments),
			}
			return nil
		})
	}

	if wantFiles && len(pr.Files.Nodes) == 0 {
		g.Go(func() error {
			files, err := FetchPRFilesREST(client, repo, pr.Number)
			if err != nil {
				return err
			}
			pr.Files.Nodes = files
			return nil
		})
	}

	if wantCommits && len(pr.Commits.Nodes) == 0 {
		g.Go(func() error {
			commits, err := FetchPRCommitsREST(client, repo, pr.Number)
			if err != nil {
				return err
			}
			pr.Commits.Nodes = commits
			pr.Commits.TotalCount = len(commits)
			return nil
		})
	}

	if wantChecks && len(pr.StatusCheckRollup.Nodes) == 0 {
		ref := pr.HeadRefOid
		if ref == "" {
			ref = pr.HeadRefName
		}
		if ref != "" {
			g.Go(func() error {
				contexts, err := FetchStatusChecksREST(client, repo, ref, false)
				if err != nil {
					return err
				}
				pr.StatusCheckRollup.Nodes = []StatusCheckRollupNode{
					{
						Commit: StatusCheckRollupCommit{
							StatusCheckRollup: CommitStatusCheckRollup{
								Contexts: CheckContexts{
									Nodes: contexts,
								},
							},
						},
					},
				}
				return nil
			})
		}
	}

	if wantReactions && len(pr.ReactionGroups) == 0 {
		g.Go(func() error {
			reactions, err := FetchPRReactionsREST(client, repo, pr.Number)
			if err != nil {
				return err
			}
			pr.ReactionGroups = reactions
			return nil
		})
	}

	return g.Wait()
}

// LookupPullRequestByNumberREST fetches a PR and hydrates requested subresource fields via REST.
func LookupPullRequestByNumberREST(client *Client, repo ghrepo.Interface, number int, fields []string) (*PullRequest, error) {
	pr, err := FetchPullRequestREST(client, repo, number)
	if err != nil {
		return nil, err
	}
	if err := HydratePullRequestFields(client, repo, pr, fields); err != nil {
		return nil, fmt.Errorf("hydrating pull request #%d: %w", number, err)
	}
	return pr, nil
}


// UnsupportedPRJSONFields lists PullRequest JSON fields that cannot be populated from REST
// and return ErrNotSupported when explicitly requested via --json.
var UnsupportedPRJSONFields = map[string]string{
	"closingIssuesReferences": "closingIssuesReferences is not supported with REST",
	"potentialMergeCommit":    "potentialMergeCommit is not supported with REST",
	"projectCards":            "projectCards is not supported with REST",
	"projectItems":            "projectItems is not supported with REST",
	"reviewDecision":          "reviewDecision is not supported with REST",
}

var singlePRDetailFields = map[string]bool{
	"additions":           true,
	"autoMergeRequest":    true,
	"changedFiles":        true,
	"comments":            true,
	"commitsCount":        true,
	"deletions":           true,
	"maintainerCanModify": true,
	"mergeStateStatus":    true,
	"mergeable":           true,
}

var prHydrationFields = map[string]bool{
	"reviews":           true,
	"latestReviews":     true,
	"comments":          true,
	"files":             true,
	"commits":           true,
	"statusCheckRollup": true,
	"reactionGroups":    true,
}

// NeedsSinglePRDetailFetch reports whether any requested field requires fetching
// the individual pull request endpoint rather than a list/search summary.
func NeedsSinglePRDetailFetch(fields []string) bool {
	for _, f := range fields {
		if singlePRDetailFields[f] {
			return true
		}
	}
	return false
}

func needsPRHydration(fields []string) bool {
	for _, f := range fields {
		if prHydrationFields[f] {
			return true
		}
	}
	return false
}

func needsSearchPRHydration(fields []string) bool {
	for _, f := range fields {
		switch f {
		case "headRefName", "headRefOid", "headRepository", "headRepositoryOwner",
			"baseRefName", "baseRefOid", "isCrossRepository", "mergedBy",
			"mergeCommit", "reviewRequests":
			return true
		default:
			if singlePRDetailFields[f] || prHydrationFields[f] {
				return true
			}
		}
	}
	return false
}

// ListPullRequestsOptions configures REST pull request listing.
type ListPullRequestsOptions struct {
	State      string
	BaseBranch string
	HeadBranch string
	Limit      int
	Fields     []string
}

// ListPullRequestsREST lists pull requests for a repository via REST GET /repos/{owner}/{repo}/pulls.
func ListPullRequestsREST(client *Client, repo ghrepo.Interface, opts ListPullRequestsOptions) (*PullRequestAndTotalCount, error) {
	var restState string
	switch opts.State {
	case "open":
		restState = "open"
	case "closed", "merged":
		restState = "closed"
	case "all":
		restState = "all"
	default:
		return nil, fmt.Errorf("invalid state: %s", opts.State)
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls")
	if err != nil {
		return nil, err
	}
	perPage := min(opts.Limit, 100)
	if opts.State == "merged" || opts.HeadBranch != "" {
		perPage = 100
	}
	path.SetQuery("per_page", strconv.Itoa(perPage))
	path.SetQuery("state", restState)
	if opts.BaseBranch != "" {
		path.SetQuery("base", opts.BaseBranch)
	}
	if opts.HeadBranch != "" {
		headQuery := opts.HeadBranch
		if !strings.Contains(headQuery, ":") {
			headQuery = fmt.Sprintf("%s:%s", repo.RepoOwner(), headQuery)
		}
		path.SetQuery("head", headQuery)
	}

	res := &PullRequestAndTotalCount{}
	seen := make(map[int]struct{})
	nextPath := path.String()

loop:
	for nextPath != "" {
		var page []restPullRequest
		next, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, err
		}

		for i := range page {
			pr := page[i].toPullRequest(repo)
			pr.Mergeable = "" // List endpoint does not include mergeability.
			if opts.State == "merged" && pr.State != "MERGED" {
				continue
			}
			if _, exists := seen[pr.Number]; exists && pr.Number > 0 {
				continue
			}
			seen[pr.Number] = struct{}{}
			res.PullRequests = append(res.PullRequests, *pr)
			if len(res.PullRequests) == opts.Limit {
				break loop
			}
		}
		nextPath = next
	}

	res.TotalCount = len(res.PullRequests)
	if err := hydratePRSlice(client, repo, res.PullRequests, opts.Fields, false); err != nil {
		return nil, err
	}
	return res, nil
}

type restSearchPullsResponse struct {
	TotalCount int `json:"total_count"`
	Items      []struct {
		restPullRequest
		PullRequest *struct {
			MergedAt *time.Time `json:"merged_at"`
		} `json:"pull_request"`
	} `json:"items"`
}

// SearchPullRequestsREST searches pull requests via REST GET /search/issues.
func SearchPullRequestsREST(client *Client, repo ghrepo.Interface, query string, limit int, fields []string) (*PullRequestAndTotalCount, error) {
	path, err := safeurl.JoinPath("search", "issues")
	if err != nil {
		return nil, err
	}
	path.SetQuery("q", query)
	path.SetQuery("per_page", strconv.Itoa(min(limit, 100)))

	res := &PullRequestAndTotalCount{SearchCapped: limit > 1000}
	seen := make(map[int]struct{})
	nextPath := path.String()

loop:
	for nextPath != "" {
		var page restSearchPullsResponse
		next, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, err
		}
		res.TotalCount = page.TotalCount

		for i := range page.Items {
			item := &page.Items[i]
			if item.PullRequest != nil && item.MergedAt == nil {
				item.MergedAt = item.PullRequest.MergedAt
			}
			pr := item.toPullRequest(repo)
			pr.Mergeable = "" // Search endpoint does not include mergeability.
			if _, exists := seen[pr.Number]; exists && pr.Number > 0 {
				continue
			}
			seen[pr.Number] = struct{}{}
			res.PullRequests = append(res.PullRequests, *pr)
			if len(res.PullRequests) == limit {
				break loop
			}
		}
		nextPath = next
	}

	if len(fields) > 0 && needsSearchPRHydration(fields) {
		if err := hydratePRSlice(client, repo, res.PullRequests, fields, true); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func hydratePRSlice(client *Client, repo ghrepo.Interface, prs []PullRequest, fields []string, forceSingleFetch bool) error {
	if len(prs) == 0 {
		return nil
	}
	fetchSingle := forceSingleFetch || NeedsSinglePRDetailFetch(fields)
	hydrateSub := needsPRHydration(fields)
	if !fetchSingle && !hydrateSub {
		return nil
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)
	for i := range prs {
		idx := i
		if prs[idx].Number == 0 {
			continue
		}
		g.Go(func() error {
			if fetchSingle {
				fullPR, err := FetchPullRequestREST(client, repo, prs[idx].Number)
				if err != nil {
					return err
				}
				prs[idx] = *fullPR
			}
			if hydrateSub {
				return HydratePullRequestFields(client, repo, &prs[idx], fields)
			}
			return nil
		})
	}
	return g.Wait()
}
