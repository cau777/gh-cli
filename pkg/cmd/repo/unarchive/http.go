package unarchive

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

func unarchiveRepo(client *http.Client, repo *api.Repository) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]bool{"archived": false})
	if err != nil {
		return err
	}
	return api.NewClientFromHTTP(client).REST(repo.RepoHost(), http.MethodPatch, path.String(), bytes.NewReader(payload), nil)
}
