package status

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"golang.org/x/sync/errgroup"
)

type requestOptions struct {
	CurrentPR      int
	HeadRef        string
	Username       string
	Fields         []string
	ConflictStatus bool

	CheckRunAndStatusContextCountsSupported bool
}

type pullRequestsPayload struct {
	ViewerCreated   api.PullRequestAndTotalCount
	ReviewRequested api.PullRequestAndTotalCount
	CurrentPR       *api.PullRequest
	DefaultBranch   string
}

func pullRequestStatus(httpClient *http.Client, repo ghrepo.Interface, options requestOptions) (*pullRequestsPayload, error) {
	apiClient := api.NewClientFromHTTP(httpClient)

	currentUsername, err := getCurrentUsername(options.Username, repo.RepoHost(), apiClient)
	if err != nil {
		return nil, err
	}

	viewerQuery := fmt.Sprintf("repo:%s state:open is:pr author:%s", ghrepo.FullName(repo), currentUsername)
	reviewerQuery := fmt.Sprintf("repo:%s state:open is:pr review-requested:%s", ghrepo.FullName(repo), currentUsername)

	viewerFields := options.Fields
	reviewerFields := options.Fields
	currentFields := options.Fields
	keepMergeable := slices.Contains(options.Fields, "mergeable")
	if len(options.Fields) == 0 {
		viewerFields = []string{"headRefName", "latestReviews", "statusCheckRollup"}
		reviewerFields = []string{"headRefName", "statusCheckRollup"}
		currentFields = []string{"latestReviews", "statusCheckRollup"}
		keepMergeable = options.ConflictStatus
	}

	payload := &pullRequestsPayload{}
	g, _ := errgroup.WithContext(context.Background())

	g.Go(func() error {
		res, err := api.SearchPullRequestsREST(apiClient, repo, viewerQuery, 10, viewerFields)
		if err != nil {
			return err
		}
		if !keepMergeable {
			clearMergeable(res.PullRequests)
		}
		payload.ViewerCreated = *res
		return nil
	})

	g.Go(func() error {
		res, err := api.SearchPullRequestsREST(apiClient, repo, reviewerQuery, 10, reviewerFields)
		if err != nil {
			return err
		}
		if !keepMergeable {
			clearMergeable(res.PullRequests)
		}
		payload.ReviewRequested = *res
		return nil
	})

	if options.CurrentPR > 0 {
		g.Go(func() error {
			pr, err := api.LookupPullRequestByNumberREST(apiClient, repo, options.CurrentPR, currentFields)
			if err != nil {
				return err
			}
			if !keepMergeable {
				pr.Mergeable = ""
			}
			payload.CurrentPR = pr
			if pr.State != "OPEN" {
				if def, err := api.RepoDefaultBranch(apiClient, repo); err == nil {
					payload.DefaultBranch = def
				}
			}
			return nil
		})
	} else if options.HeadRef != "" {
		g.Go(func() error {
			headQuery := options.HeadRef
			if !strings.Contains(headQuery, ":") {
				headQuery = fmt.Sprintf("%s:%s", repo.RepoOwner(), headQuery)
			}
			prs, def, err := api.FetchPullRequestsForBranchREST(apiClient, repo, headQuery, nil)
			if err != nil {
				return err
			}
			payload.DefaultBranch = def
			for i := range prs {
				if prs[i].HeadLabel() == options.HeadRef {
					pr, err := api.LookupPullRequestByNumberREST(apiClient, repo, prs[i].Number, currentFields)
					if err != nil {
						return err
					}
					if !keepMergeable {
						pr.Mergeable = ""
					}
					payload.CurrentPR = pr
					break
				}
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return payload, nil
}

func clearMergeable(prs []api.PullRequest) {
	for i := range prs {
		prs[i].Mergeable = ""
	}
}

func getCurrentUsername(username string, hostname string, apiClient *api.Client) (string, error) {
	if username == "@me" && ghauth.IsEnterprise(hostname) {
		var err error
		username, err = api.CurrentLoginName(apiClient, hostname)
		if err != nil {
			return "", err
		}
	}
	return username, nil
}
