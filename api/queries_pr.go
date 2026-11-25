package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type PullRequestAndTotalCount struct {
	TotalCount   int
	PullRequests []PullRequest
	SearchCapped bool
}

type PullRequestMergeable string

const (
	PullRequestMergeableConflicting PullRequestMergeable = "CONFLICTING"
	PullRequestMergeableMergeable   PullRequestMergeable = "MERGEABLE"
	PullRequestMergeableUnknown     PullRequestMergeable = "UNKNOWN"
)

type PullRequest struct {
	ID                  string
	FullDatabaseID      string
	Number              int
	Title               string
	State               string
	Closed              bool
	URL                 string
	BaseRefName         string
	BaseRefOid          string
	HeadRefName         string
	HeadRefOid          string
	Body                string
	Mergeable           PullRequestMergeable
	Additions           int
	Deletions           int
	ChangedFiles        int
	MergeStateStatus    string
	IsInMergeQueue      bool
	IsMergeQueueEnabled bool // Indicates whether the pull request's base ref has a merge queue enabled.
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ClosedAt            *time.Time
	MergedAt            *time.Time

	AutoMergeRequest *AutoMergeRequest

	MergeCommit          *Commit
	PotentialMergeCommit *Commit

	Files struct {
		Nodes []PullRequestFile
	}

	Author              Author
	MergedBy            *Author
	HeadRepositoryOwner Owner
	HeadRepository      *PRRepository
	Repository          *PRRepository
	IsCrossRepository   bool
	IsDraft             bool
	MaintainerCanModify bool

	BaseRef struct {
		BranchProtectionRule struct {
			RequiresStrictStatusChecks   bool
			RequiredApprovingReviewCount int
		}
	}

	ReviewDecision string

	Commits struct {
		TotalCount int
		Nodes      []PullRequestCommit
	}
	StatusCheckRollup struct {
		Nodes []StatusCheckRollupNode
	}

	Assignees      Assignees
	AssignedActors AssignedActors
	Labels         Labels
	ProjectCards   ProjectCards
	ProjectItems   ProjectItems
	Milestone      *Milestone
	Comments       Comments
	ReactionGroups ReactionGroups
	Reviews        PullRequestReviews
	LatestReviews  PullRequestReviews
	ReviewRequests ReviewRequests

	ClosingIssuesReferences ClosingIssuesReferences
}

type StatusCheckRollupNode struct {
	Commit StatusCheckRollupCommit
}

type StatusCheckRollupCommit struct {
	StatusCheckRollup CommitStatusCheckRollup
}

type CommitStatusCheckRollup struct {
	Contexts CheckContexts
}

type ClosingIssuesReferences struct {
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

// https://docs.github.com/en/graphql/reference/enums#checkrunstate
type CheckRunState string

const (
	CheckRunStateActionRequired CheckRunState = "ACTION_REQUIRED"
	CheckRunStateCancelled      CheckRunState = "CANCELLED"
	CheckRunStateCompleted      CheckRunState = "COMPLETED"
	CheckRunStateFailure        CheckRunState = "FAILURE"
	CheckRunStateInProgress     CheckRunState = "IN_PROGRESS"
	CheckRunStateNeutral        CheckRunState = "NEUTRAL"
	CheckRunStatePending        CheckRunState = "PENDING"
	CheckRunStateQueued         CheckRunState = "QUEUED"
	CheckRunStateSkipped        CheckRunState = "SKIPPED"
	CheckRunStateStale          CheckRunState = "STALE"
	CheckRunStateStartupFailure CheckRunState = "STARTUP_FAILURE"
	CheckRunStateSuccess        CheckRunState = "SUCCESS"
	CheckRunStateTimedOut       CheckRunState = "TIMED_OUT"
	CheckRunStateWaiting        CheckRunState = "WAITING"
)

type CheckRunCountByState struct {
	State CheckRunState
	Count int
}

// https://docs.github.com/en/graphql/reference/enums#statusstate
type StatusState string

const (
	StatusStateError    StatusState = "ERROR"
	StatusStateExpected StatusState = "EXPECTED"
	StatusStateFailure  StatusState = "FAILURE"
	StatusStatePending  StatusState = "PENDING"
	StatusStateSuccess  StatusState = "SUCCESS"
)

type StatusContextCountByState struct {
	State StatusState
	Count int
}

// https://docs.github.com/en/graphql/reference/enums#checkstatusstate
type CheckStatusState string

const (
	CheckStatusStateCompleted  CheckStatusState = "COMPLETED"
	CheckStatusStateInProgress CheckStatusState = "IN_PROGRESS"
	CheckStatusStatePending    CheckStatusState = "PENDING"
	CheckStatusStateQueued     CheckStatusState = "QUEUED"
	CheckStatusStateRequested  CheckStatusState = "REQUESTED"
	CheckStatusStateWaiting    CheckStatusState = "WAITING"
)

// https://docs.github.com/en/graphql/reference/enums#checkconclusionstate
type CheckConclusionState string

const (
	CheckConclusionStateActionRequired CheckConclusionState = "ACTION_REQUIRED"
	CheckConclusionStateCancelled      CheckConclusionState = "CANCELLED"
	CheckConclusionStateFailure        CheckConclusionState = "FAILURE"
	CheckConclusionStateNeutral        CheckConclusionState = "NEUTRAL"
	CheckConclusionStateSkipped        CheckConclusionState = "SKIPPED"
	CheckConclusionStateStale          CheckConclusionState = "STALE"
	CheckConclusionStateStartupFailure CheckConclusionState = "STARTUP_FAILURE"
	CheckConclusionStateSuccess        CheckConclusionState = "SUCCESS"
	CheckConclusionStateTimedOut       CheckConclusionState = "TIMED_OUT"
)

type CheckContexts struct {
	// These fields are available on newer versions of the GraphQL API
	// to support summary counts by state
	CheckRunCount              int
	CheckRunCountsByState      []CheckRunCountByState
	StatusContextCount         int
	StatusContextCountsByState []StatusContextCountByState

	// These are available on older versions and provide more details
	// required for checks
	Nodes    []CheckContext
	PageInfo struct {
		HasNextPage bool
		EndCursor   string
	}
}

type CheckContext struct {
	TypeName   string     `json:"__typename"`
	Name       string     `json:"name"`
	IsRequired bool       `json:"isRequired"`
	CheckSuite CheckSuite `json:"checkSuite"`
	// QUEUED IN_PROGRESS COMPLETED WAITING PENDING REQUESTED
	Status string `json:"status"`
	// ACTION_REQUIRED TIMED_OUT CANCELLED FAILURE SUCCESS NEUTRAL SKIPPED STARTUP_FAILURE STALE
	Conclusion  CheckConclusionState `json:"conclusion"`
	StartedAt   time.Time            `json:"startedAt"`
	CompletedAt time.Time            `json:"completedAt"`
	DetailsURL  string               `json:"detailsUrl"`

	/* StatusContext fields */
	Context     string `json:"context"`
	Description string `json:"description"`
	// EXPECTED ERROR FAILURE PENDING SUCCESS
	State     StatusState `json:"state"`
	TargetURL string      `json:"targetUrl"`
	CreatedAt time.Time   `json:"createdAt"`
}

type CheckSuite struct {
	WorkflowRun WorkflowRun `json:"workflowRun"`
}

type WorkflowRun struct {
	Event    string   `json:"event"`
	Workflow Workflow `json:"workflow"`
}

type Workflow struct {
	Name string `json:"name"`
}

type PRRepository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	NameWithOwner string `json:"nameWithOwner"`

	// Fields below are omitted when empty because this type is used by
	// headRepository.

	DatabaseID int64 `json:"databaseId,omitempty"`
	// One of ADMIN, MAINTAIN, WRITE, TRIAGE, READ.
	ViewerPermission string `json:"viewerPermission,omitempty"`
}

type AutoMergeRequest struct {
	AuthorEmail    *string `json:"authorEmail"`
	CommitBody     *string `json:"commitBody"`
	CommitHeadline *string `json:"commitHeadline"`
	// MERGE, REBASE, SQUASH
	MergeMethod string    `json:"mergeMethod"`
	EnabledAt   time.Time `json:"enabledAt"`
	EnabledBy   Author    `json:"enabledBy"`
}

// Commit loads just the commit SHA and nothing else
type Commit struct {
	OID string `json:"oid"`
}

type PullRequestCommit struct {
	Commit PullRequestCommitCommit
}

// PullRequestCommitCommit contains full information about a commit
type PullRequestCommitCommit struct {
	OID     string `json:"oid"`
	Authors struct {
		Nodes []struct {
			Name  string
			Email string
			User  GitHubUser
		}
	}
	MessageHeadline string
	MessageBody     string
	CommittedDate   time.Time
	AuthoredDate    time.Time
}

type PullRequestFile struct {
	Path       string `json:"path"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	ChangeType string `json:"changeType"`
}

func (pr PullRequest) HeadLabel() string {
	if pr.IsCrossRepository {
		return fmt.Sprintf("%s:%s", pr.HeadRepositoryOwner.Login, pr.HeadRefName)
	}
	return pr.HeadRefName
}

func (pr PullRequest) Link() string {
	return pr.URL
}

func (pr PullRequest) Identifier() string {
	return pr.ID
}

func (pr PullRequest) CurrentUserComments() []Comment {
	return pr.Comments.CurrentUserComments()
}

// RepositoryDatabaseID returns the numeric REST id of the repository, or zero
// when the "repository" field was not requested.
func (pr PullRequest) RepositoryDatabaseID() int64 {
	if pr.Repository == nil {
		return 0
	}
	return pr.Repository.DatabaseID
}

// RepositoryViewerPermission returns the requesting user's role on the
// repository, or an empty string when the "repository" field was not requested.
func (pr PullRequest) RepositoryViewerPermission() string {
	if pr.Repository == nil {
		return ""
	}
	return pr.Repository.ViewerPermission
}

func (pr PullRequest) IsOpen() bool {
	return pr.State == "OPEN"
}

type PullRequestChecksStatus struct {
	Pending int
	Failing int
	Passing int
	Total   int
}

func (pr *PullRequest) ChecksStatus() PullRequestChecksStatus {
	var summary PullRequestChecksStatus

	if len(pr.StatusCheckRollup.Nodes) == 0 {
		return summary
	}

	contexts := pr.StatusCheckRollup.Nodes[0].Commit.StatusCheckRollup.Contexts

	// If this commit has counts by state then we can summarise check status from those
	if len(contexts.CheckRunCountsByState) != 0 && len(contexts.StatusContextCountsByState) != 0 {
		summary.Total = contexts.CheckRunCount + contexts.StatusContextCount
		for _, countByState := range contexts.CheckRunCountsByState {
			switch parseCheckStatusFromCheckRunState(countByState.State) {
			case passing:
				summary.Passing += countByState.Count
			case failing:
				summary.Failing += countByState.Count
			default:
				summary.Pending += countByState.Count
			}
		}

		for _, countByState := range contexts.StatusContextCountsByState {
			switch parseCheckStatusFromStatusState(countByState.State) {
			case passing:
				summary.Passing += countByState.Count
			case failing:
				summary.Failing += countByState.Count
			default:
				summary.Pending += countByState.Count
			}
		}

		return summary
	}

	// If we don't have the counts by state, then we'll need to summarise by looking at the more detailed contexts
	for _, c := range contexts.Nodes {
		// Nodes are a discriminated union of CheckRun or StatusContext and we can match on
		// the TypeName to narrow the type.
		if c.TypeName == "CheckRun" {
			// https://docs.github.com/en/graphql/reference/enums#checkstatusstate
			// If the status is completed then we can check the conclusion field
			if c.Status == "COMPLETED" {
				switch parseCheckStatusFromCheckConclusionState(c.Conclusion) {
				case passing:
					summary.Passing++
				case failing:
					summary.Failing++
				default:
					summary.Pending++
				}
				// otherwise we're in some form of pending state:
				// "COMPLETED", "IN_PROGRESS", "PENDING", "QUEUED", "REQUESTED", "WAITING" or otherwise unknown
			} else {
				summary.Pending++
			}

		} else { // c.TypeName == StatusContext
			switch parseCheckStatusFromStatusState(c.State) {
			case passing:
				summary.Passing++
			case failing:
				summary.Failing++
			default:
				summary.Pending++
			}
		}
		summary.Total++
	}

	return summary
}

type checkStatus int

const (
	passing checkStatus = iota
	failing
	pending
)

func parseCheckStatusFromStatusState(state StatusState) checkStatus {
	switch state {
	case StatusStateSuccess:
		return passing
	case StatusStateFailure, StatusStateError:
		return failing
	case StatusStateExpected, StatusStatePending:
		return pending
	// Currently, we treat anything unknown as pending, which includes any future unknown
	// states we might get back from the API. It might be interesting to do some work to add an additional
	// unknown state.
	default:
		return pending
	}
}

func parseCheckStatusFromCheckRunState(state CheckRunState) checkStatus {
	switch state {
	case CheckRunStateNeutral, CheckRunStateSkipped, CheckRunStateSuccess:
		return passing
	case CheckRunStateActionRequired, CheckRunStateCancelled, CheckRunStateFailure, CheckRunStateTimedOut:
		return failing
	case CheckRunStateCompleted, CheckRunStateInProgress, CheckRunStatePending, CheckRunStateQueued,
		CheckRunStateStale, CheckRunStateStartupFailure, CheckRunStateWaiting:
		return pending
	// Currently, we treat anything unknown as pending, which includes any future unknown
	// states we might get back from the API. It might be interesting to do some work to add an additional
	// unknown state.
	default:
		return pending
	}
}

func parseCheckStatusFromCheckConclusionState(state CheckConclusionState) checkStatus {
	switch state {
	case CheckConclusionStateNeutral, CheckConclusionStateSkipped, CheckConclusionStateSuccess:
		return passing
	case CheckConclusionStateActionRequired, CheckConclusionStateCancelled, CheckConclusionStateFailure, CheckConclusionStateTimedOut:
		return failing
	case CheckConclusionStateStale, CheckConclusionStateStartupFailure:
		return pending
	// Currently, we treat anything unknown as pending, which includes any future unknown
	// states we might get back from the API. It might be interesting to do some work to add an additional
	// unknown state.
	default:
		return pending
	}
}

// CreatePullRequest creates a pull request in a GitHub repository
func CreatePullRequest(client *Client, repo *Repository, params map[string]any) (*PullRequest, error) {
	if ids, ok := params["projectIds"].([]string); ok && len(ids) > 0 {
		return nil, NotSupportedError("adding a pull request to projects is not supported with REST")
	}
	if ids, ok := params["projectV2Ids"].([]string); ok && len(ids) > 0 {
		return nil, NotSupportedError("adding a pull request to projects is not supported with REST")
	}
	if bots, ok := params["botReviewerLogins"].([]string); ok && len(bots) > 0 {
		return nil, NotSupportedError("requesting bot reviewers is not supported with REST")
	}
	if logins, ok := params["assigneeLogins"].([]string); ok {
		for _, login := range logins {
			if login == CopilotAssigneeLogin {
				return nil, NotSupportedError("assigning Copilot is not supported with REST")
			}
		}
	}

	reqBody := struct {
		Title               any `json:"title,omitempty"`
		Body                any `json:"body,omitempty"`
		Head                any `json:"head"`
		Base                any `json:"base"`
		Draft               any `json:"draft,omitempty"`
		MaintainerCanModify any `json:"maintainer_can_modify,omitempty"`
	}{
		Title:               params["title"],
		Body:                params["body"],
		Head:                params["headRefName"],
		Base:                params["baseRefName"],
		Draft:               params["draft"],
		MaintainerCanModify: params["maintainerCanModify"],
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	createPath, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls")
	if err != nil {
		return nil, err
	}

	var created struct {
		NodeID  string `json:"node_id"`
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := client.REST(repo.RepoHost(), http.MethodPost, createPath.String(), bytes.NewReader(payload), &created); err != nil {
		return nil, err
	}

	pr := &PullRequest{
		ID:     created.NodeID,
		Number: created.Number,
		URL:    created.HTMLURL,
	}

	issuePatch := make(map[string]any)
	if logins, ok := params["assigneeLogins"].([]string); ok && len(logins) > 0 {
		issuePatch["assignees"] = logins
	}
	if labels, ok := params["labelNames"].([]string); ok && len(labels) > 0 {
		issuePatch["labels"] = labels
	}
	if milestoneNum, ok := params["milestoneNumber"].(int); ok && milestoneNum > 0 {
		issuePatch["milestone"] = milestoneNum
	}

	if len(issuePatch) > 0 {
		patchPayload, err := json.Marshal(issuePatch)
		if err != nil {
			return pr, err
		}
		issuePath, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(pr.Number))
		if err != nil {
			return pr, err
		}
		if err := client.REST(repo.RepoHost(), http.MethodPatch, issuePath.String(), bytes.NewReader(patchPayload), nil); err != nil {
			return pr, err
		}
	}

	userLogins, _ := params["userReviewerLogins"].([]string)
	teamSlugs, _ := params["teamReviewerSlugs"].([]string)
	if len(userLogins) > 0 || len(teamSlugs) > 0 {
		if err := AddPullRequestReviews(client, repo, pr.Number, userLogins, teamSlugs); err != nil {
			return pr, err
		}
	}

	return pr, nil
}

// UpdatePullRequestBranch updates a pull request branch by merging its base
// branch using the REST API.
func UpdatePullRequestBranch(client *Client, repo ghrepo.Interface, prNumber int, expectedHeadOid string) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls", strconv.Itoa(prNumber), "update-branch")
	if err != nil {
		return err
	}

	body := struct {
		ExpectedHeadSHA string `json:"expected_head_sha,omitempty"`
	}{
		ExpectedHeadSHA: expectedHeadOid,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	return client.REST(repo.RepoHost(), http.MethodPut, path.String(), bytes.NewReader(payload), nil)
}

func PullRequestClose(httpClient *http.Client, repo ghrepo.Interface, prNumber int) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls", strconv.Itoa(prNumber))
	if err != nil {
		return err
	}

	client := NewClientFromHTTP(httpClient)
	return client.REST(repo.RepoHost(), http.MethodPatch, path.String(), bytes.NewBufferString(`{"state":"closed"}`), nil)
}

func PullRequestReopen(httpClient *http.Client, repo ghrepo.Interface, prNumber int) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls", strconv.Itoa(prNumber))
	if err != nil {
		return err
	}

	client := NewClientFromHTTP(httpClient)
	return client.REST(repo.RepoHost(), http.MethodPatch, path.String(), bytes.NewBufferString(`{"state":"open"}`), nil)
}

func PullRequestReady(client *Client, repo ghrepo.Interface, pr *PullRequest) error {
	return NotSupportedError("marking a pull request ready for review is not supported with REST")
}

// PullRequestRevertInput captures parameters for reverting a pull request.
type PullRequestRevertInput struct {
	PullRequestID string
	Draft         bool
	Body          *string
	Title         *string
}

func PullRequestRevert(client *Client, repo ghrepo.Interface, params PullRequestRevertInput) (*PullRequest, error) {
	return nil, NotSupportedError("reverting a pull request is not supported with REST")
}

func ConvertPullRequestToDraft(client *Client, repo ghrepo.Interface, pr *PullRequest) error {
	return NotSupportedError("converting a pull request to draft is not supported with REST")
}

func BranchDeleteRemote(client *Client, repo ghrepo.Interface, branch string) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "git", "refs", fmt.Sprintf("heads/%s", branch))
	if err != nil {
		return err
	}
	return client.REST(repo.RepoHost(), "DELETE", path.String(), nil, nil)
}

type RefComparison struct {
	AheadBy  int
	BehindBy int
	Status   string
}

// ComparePullRequestBaseBranchWith compares baseRef against headRef via REST.
func ComparePullRequestBaseBranchWith(client *Client, repo ghrepo.Interface, baseRef, headRef string) (*RefComparison, error) {
	if baseRef == "" {
		return nil, errors.New("base branch ref is required for REST comparison")
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "compare", fmt.Sprintf("%s...%s", baseRef, headRef))
	if err != nil {
		return nil, err
	}

	var resp struct {
		AheadBy  int    `json:"ahead_by"`
		BehindBy int    `json:"behind_by"`
		Status   string `json:"status"`
	}
	if err := client.REST(repo.RepoHost(), http.MethodGet, path.String(), nil, &resp); err != nil {
		return nil, err
	}

	return &RefComparison{
		AheadBy:  resp.AheadBy,
		BehindBy: resp.BehindBy,
		Status:   resp.Status,
	}, nil
}
