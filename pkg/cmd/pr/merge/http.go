package merge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type PullRequestMergeMethod int

const (
	PullRequestMergeMethodMerge PullRequestMergeMethod = iota
	PullRequestMergeMethodRebase
	PullRequestMergeMethodSquash
)

const (
	MergeStateStatusBehind   = "BEHIND"
	MergeStateStatusBlocked  = "BLOCKED"
	MergeStateStatusClean    = "CLEAN"
	MergeStateStatusDirty    = "DIRTY"
	MergeStateStatusHasHooks = "HAS_HOOKS"
	MergeStateStatusMerged   = "MERGED"
	MergeStateStatusUnstable = "UNSTABLE"
)

type mergePayload struct {
	repo              ghrepo.Interface
	pullRequestID     string
	pullRequestNumber int
	prTitle           string
	prBody            string
	headRefName       string
	method            PullRequestMergeMethod
	auto              bool
	commitSubject     string
	commitBody        string
	setCommitBody     bool
	expectedHeadOid   string
	authorEmail       string
}

func mergePullRequest(client *http.Client, payload mergePayload) error {
	if payload.auto {
		return api.NotSupportedError("auto-merge and merge queue are not supported with REST")
	}
	if payload.authorEmail != "" {
		return api.NotSupportedError("setting commit author email (--author-email) is not supported with REST")
	}

	var method string
	switch payload.method {
	case PullRequestMergeMethodMerge:
		method = "merge"
	case PullRequestMergeMethodRebase:
		method = "rebase"
	case PullRequestMergeMethodSquash:
		method = "squash"
	}

	path, err := safeurl.JoinPath("repos", payload.repo.RepoOwner(), payload.repo.RepoName(), "pulls", strconv.Itoa(payload.pullRequestNumber), "merge")
	if err != nil {
		return err
	}

	reqBody := struct {
		MergeMethod   string  `json:"merge_method,omitempty"`
		CommitTitle   string  `json:"commit_title,omitempty"`
		CommitMessage *string `json:"commit_message,omitempty"`
		SHA           string  `json:"sha,omitempty"`
	}{
		MergeMethod: method,
		CommitTitle: payload.commitSubject,
		SHA:         payload.expectedHeadOid,
	}
	if payload.setCommitBody {
		reqBody.CommitMessage = &payload.commitBody
	}

	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(client)
	return apiClient.REST(payload.repo.RepoHost(), http.MethodPut, path.String(), bytes.NewReader(encoded), nil)
}

func disableAutoMerge(client *http.Client, repo ghrepo.Interface, prID string) error {
	return api.NotSupportedError("disabling auto-merge is not supported with REST")
}

func getMergeText(client *http.Client, payload mergePayload) (string, string, error) {
	switch payload.method {
	case PullRequestMergeMethodMerge:
		subject := fmt.Sprintf("Merge pull request #%d from %s", payload.pullRequestNumber, payload.headRefName)
		return subject, payload.prTitle, nil
	case PullRequestMergeMethodSquash:
		subject := fmt.Sprintf("%s (#%d)", payload.prTitle, payload.pullRequestNumber)
		apiClient := api.NewClientFromHTTP(client)
		commits, err := api.FetchPRCommitsREST(apiClient, payload.repo, payload.pullRequestNumber)
		if err != nil {
			return subject, payload.prBody, nil
		}
		var lines []string
		for _, c := range commits {
			if c.Commit.MessageHeadline != "" {
				lines = append(lines, "* "+c.Commit.MessageHeadline)
			}
		}
		if len(lines) > 0 {
			return subject, strings.Join(lines, "\n"), nil
		}
		return subject, payload.prBody, nil
	default:
		return "", "", nil
	}
}
