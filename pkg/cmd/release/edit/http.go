package edit

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
)

func editRelease(httpClient *http.Client, repo ghrepo.Interface, releaseID int64, params map[string]any) (*shared.Release, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", strconv.FormatInt(releaseID, 10))
	if err != nil {
		return nil, err
	}

	var newRelease shared.Release
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	if err := api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), http.MethodPatch, path.String(), bytes.NewBuffer(bodyBytes), &newRelease); err != nil {
		return nil, err
	}

	return &newRelease, nil
}

func remoteTagExists(httpClient *http.Client, repo ghrepo.Interface, tagName string) (bool, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "git", "ref", "tags", tagName)
	if err != nil {
		return false, err
	}

	err = api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), http.MethodGet, path.String(), nil, nil)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
