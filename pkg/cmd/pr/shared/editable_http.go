package shared

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"golang.org/x/sync/errgroup"
)

// UpdatePullRequestREST updates a pull request and its issue metadata using REST.
func UpdatePullRequestREST(httpClient *http.Client, repo ghrepo.Interface, number int, options Editable) error {
	if options.Projects.Edited {
		return api.NotSupportedError("editing pull request projects is not supported with REST")
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	var wg errgroup.Group

	if options.Labels.Edited {
		syncLabelsREST(&wg, apiClient, repo, number, options)
	}

	if options.Title.Edited || options.Body.Edited || options.Base.Edited {
		wg.Go(func() error {
			prPatch := make(map[string]any)
			if options.Title.Edited {
				prPatch["title"] = options.Title.Value
			}
			if options.Body.Edited {
				prPatch["body"] = options.Body.Value
			}
			if options.Base.Edited {
				prPatch["base"] = options.Base.Value
			}
			payload, err := json.Marshal(prPatch)
			if err != nil {
				return err
			}
			path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls", strconv.Itoa(number))
			if err != nil {
				return err
			}
			return apiClient.REST(repo.RepoHost(), http.MethodPatch, path.String(), bytes.NewReader(payload), nil)
		})
	}

	if options.Assignees.Edited || options.Milestone.Edited {
		wg.Go(func() error {
			return patchIssueFieldsREST(apiClient, repo, number, false, options)
		})
	}

	return wg.Wait()
}

// UpdateIssueREST updates an issue's fields, labels, assignees, and milestone using REST.
func UpdateIssueREST(httpClient *http.Client, repo ghrepo.Interface, number int, options Editable) error {
	if options.Projects.Edited {
		return api.NotSupportedError("editing projects on an issue is not supported with REST")
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	var wg errgroup.Group

	if options.Labels.Edited {
		syncLabelsREST(&wg, apiClient, repo, number, options)
	}

	if options.Title.Edited || options.Body.Edited || options.Assignees.Edited || options.Milestone.Edited {
		wg.Go(func() error {
			return patchIssueFieldsREST(apiClient, repo, number, true, options)
		})
	}

	return wg.Wait()
}

func syncLabelsREST(wg *errgroup.Group, apiClient *api.Client, repo ghrepo.Interface, number int, options Editable) {
	if len(options.Labels.Add) > 0 {
		wg.Go(func() error {
			labels := options.Labels.Add
			if len(options.Metadata.Labels) > 0 {
				var err error
				labels, err = options.Metadata.LabelsToNames(labels)
				if err != nil {
					return err
				}
			}
			path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(number), "labels")
			if err != nil {
				return err
			}
			payload, err := json.Marshal(struct {
				Labels []string `json:"labels"`
			}{Labels: labels})
			if err != nil {
				return err
			}
			return apiClient.REST(repo.RepoHost(), http.MethodPost, path.String(), bytes.NewReader(payload), nil)
		})
	}
	if len(options.Labels.Remove) > 0 {
		wg.Go(func() error {
			labels := options.Labels.Remove
			if len(options.Metadata.Labels) > 0 {
				var err error
				labels, err = options.Metadata.LabelsToNames(labels)
				if err != nil {
					return err
				}
			}
			for _, label := range labels {
				path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(number), "labels", label)
				if err != nil {
					return err
				}
				err = apiClient.REST(repo.RepoHost(), http.MethodDelete, path.String(), nil, nil)
				if err != nil {
					var httpErr api.HTTPError
					if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
						continue
					}
					return err
				}
			}
			return nil
		})
	}
}

func patchIssueFieldsREST(apiClient *api.Client, repo ghrepo.Interface, number int, includeTitleBody bool, options Editable) error {
	issuePatch := make(map[string]any)
	if includeTitleBody {
		if options.Title.Edited {
			issuePatch["title"] = options.Title.Value
		}
		if options.Body.Edited {
			issuePatch["body"] = options.Body.Value
		}
	}
	if options.Assignees.Edited {
		logins, err := options.AssigneeLogins(apiClient, repo)
		if err != nil {
			return err
		}
		for _, login := range logins {
			if strings.EqualFold(login, "@copilot") || login == api.CopilotAssigneeLogin {
				return api.NotSupportedError("assigning Copilot is not supported with REST")
			}
		}
		if logins == nil {
			logins = []string{}
		}
		issuePatch["assignees"] = logins
	}
	if options.Milestone.Edited {
		if options.Milestone.Value == noMilestone || options.Milestone.Value == "" {
			issuePatch["milestone"] = nil
		} else {
			num, err := options.Metadata.MilestoneToNumber(options.Milestone.Value)
			if err != nil {
				return err
			}
			issuePatch["milestone"] = num
		}
	}
	payload, err := json.Marshal(issuePatch)
	if err != nil {
		return err
	}
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(number))
	if err != nil {
		return err
	}
	return apiClient.REST(repo.RepoHost(), http.MethodPatch, path.String(), bytes.NewReader(payload), nil)
}
