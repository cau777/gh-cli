package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type restCheckRunsResponse struct {
	TotalCount int            `json:"total_count"`
	CheckRuns  []restCheckRun `json:"check_runs"`
}

type restCheckRun struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt time.Time  `json:"completed_at"`
	HTMLURL     string     `json:"html_url"`
	DetailsURL  string     `json:"details_url"`
	CheckSuite  *restSuite `json:"check_suite"`
	App         *restApp   `json:"app"`
}

type restSuite struct {
	ID int64 `json:"id"`
}

type restApp struct {
	Slug string `json:"slug"`
}

type restCombinedStatusResponse struct {
	State      string               `json:"state"`
	TotalCount int                  `json:"total_count"`
	Statuses   []restCommitStatus   `json:"statuses"`
}

type restCommitStatus struct {
	Context     string    `json:"context"`
	Description string    `json:"description"`
	State       string    `json:"state"`
	TargetURL   string    `json:"target_url"`
	CreatedAt   time.Time `json:"created_at"`
}

type restWorkflowRunsResponse struct {
	WorkflowRuns []restWorkflowRun `json:"workflow_runs"`
}

type restWorkflowRun struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Event        string `json:"event"`
	CheckSuiteID int64  `json:"check_suite_id"`
}

// FetchStatusChecksREST fetches check runs and commit statuses for a git ref via REST
// and returns them normalized into CheckContext entries matching the GraphQL shape.
// When any check run references a check_suite.id, it also queries Actions workflow runs
// for head_sha to populate Workflow.Name and (when includeEvent is true) Event.
func FetchStatusChecksREST(client *Client, repo ghrepo.Interface, ref string, includeEvent bool) ([]CheckContext, error) {
	runsPath, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "commits", ref, "check-runs")
	if err != nil {
		return nil, err
	}
	runsPath.SetQuery("per_page", "100")

	var checkRuns []restCheckRun
	nextPath := runsPath.String()
	for nextPath != "" {
		var page restCheckRunsResponse
		next, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, err
		}
		checkRuns = append(checkRuns, page.CheckRuns...)
		nextPath = next
	}

	statusPath, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "commits", ref, "status")
	if err != nil {
		return nil, err
	}
	statusPath.SetQuery("per_page", "100")

	var commitStatuses []restCommitStatus
	nextPath = statusPath.String()
	for nextPath != "" {
		var page restCombinedStatusResponse
		next, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, err
		}
		commitStatuses = append(commitStatuses, page.Statuses...)
		nextPath = next
	}

	suiteRuns := map[int64]restWorkflowRun{}
	if hasCheckSuiteID(checkRuns) {
		suiteRuns, _ = fetchWorkflowRunsBySuite(client, repo, ref)
	}

	contexts := make([]CheckContext, 0, len(checkRuns)+len(commitStatuses))
	for _, cr := range checkRuns {
		detailsURL := cr.DetailsURL
		if detailsURL == "" {
			detailsURL = cr.HTMLURL
		}
		ctx := CheckContext{
			TypeName:    "CheckRun",
			Name:        cr.Name,
			Status:      strings.ToUpper(cr.Status),
			Conclusion:  CheckConclusionState(strings.ToUpper(cr.Conclusion)),
			StartedAt:   cr.StartedAt,
			CompletedAt: cr.CompletedAt,
			DetailsURL:  detailsURL,
		}
		if cr.CheckSuite != nil {
			if wfRun, ok := suiteRuns[cr.CheckSuite.ID]; ok {
				ctx.CheckSuite.WorkflowRun.Workflow.Name = wfRun.Name
				if includeEvent {
					ctx.CheckSuite.WorkflowRun.Event = wfRun.Event
				}
			}
		}
		contexts = append(contexts, ctx)
	}

	for _, st := range commitStatuses {
		contexts = append(contexts, CheckContext{
			TypeName:    "StatusContext",
			Context:     st.Context,
			Description: st.Description,
			State:       StatusState(strings.ToUpper(st.State)),
			TargetURL:   st.TargetURL,
			CreatedAt:   st.CreatedAt,
		})
	}

	return contexts, nil
}

func hasCheckSuiteID(checkRuns []restCheckRun) bool {
	for _, cr := range checkRuns {
		if cr.CheckSuite != nil && cr.CheckSuite.ID != 0 {
			return true
		}
	}
	return false
}

func fetchWorkflowRunsBySuite(client *Client, repo ghrepo.Interface, headSHA string) (map[int64]restWorkflowRun, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "runs")
	if err != nil {
		return nil, err
	}
	path.SetQuery("head_sha", headSHA)
	path.SetQuery("per_page", "100")

	out := make(map[int64]restWorkflowRun)
	nextPath := path.String()
	for nextPath != "" {
		var page restWorkflowRunsResponse
		next, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return out, fmt.Errorf("fetching workflow runs: %w", err)
		}
		for _, r := range page.WorkflowRuns {
			if r.CheckSuiteID != 0 {
				out[r.CheckSuiteID] = r
			}
		}
		nextPath = next
	}
	return out, nil
}
