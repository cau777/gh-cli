package readdir

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

// Git object modes. The high bits encode the object type (modeTypeMask); regular
// files additionally carry permission bits, so they are matched against modeFile
// after masking rather than by an exact value.
const (
	modeTypeMask  = 0o170000
	modeDir       = 0o040000
	modeFile      = 0o100000
	modeSymlink   = 0o120000
	modeSubmodule = 0o160000
)

// repoDir is a resolved directory listing for a single tree path.
type repoDir struct {
	GitSHA  string
	ID      string
	Entries []dirEntry
}

// dirEntry is a single entry within a directory listing.
type dirEntry struct {
	Name      string
	Path      string
	NameRaw   string
	PathRaw   string
	Type      string
	GitType   string
	Mode      int
	GitSHA    string
	Size      int
	Submodule *submodule
}

// submodule holds the extra metadata GraphQL exposes for a submodule entry.
type submodule struct {
	GitURL              string
	Branch              *string
	SubprojectCommitOid string
}

// isExecutable reports whether the entry is a regular file with an executable bit set.
func (e dirEntry) isExecutable() bool {
	return e.Type == "file" && e.Mode&0o111 != 0
}

// modeOctal renders the git mode as a six-digit octal string (e.g. 100644).
func (e dirEntry) modeOctal() string {
	return fmt.Sprintf("%06o", e.Mode)
}

// ExportData implements the cmdutil exportable interface for a single entry.
func (e dirEntry) ExportData(fields []string) map[string]any {
	data := map[string]any{}
	for _, field := range fields {
		switch field {
		case "name":
			data[field] = e.Name
		case "path":
			data[field] = e.Path
		case "nameRaw":
			data[field] = e.NameRaw
		case "pathRaw":
			data[field] = e.PathRaw
		case "type":
			data[field] = e.Type
		case "gitType":
			data[field] = e.GitType
		case "mode":
			data[field] = e.Mode
		case "modeOctal":
			data[field] = e.modeOctal()
		case "gitSHA":
			data[field] = e.GitSHA
		case "size":
			data[field] = e.Size
		case "submodule":
			if e.Submodule == nil {
				data[field] = nil
			} else {
				data[field] = map[string]any{
					"gitUrl":              e.Submodule.GitURL,
					"branch":              e.Submodule.Branch,
					"subprojectCommitOid": e.Submodule.SubprojectCommitOid,
				}
			}
		}
	}
	return data
}

// ExportData implements the cmdutil exportable interface for the directory.
//
// gitSHA and id are structural and always present; the requested fields select
// which properties appear on each entry.
func (d *repoDir) ExportData(fields []string) map[string]any {
	entries := make([]any, 0, len(d.Entries))
	for _, e := range d.Entries {
		entries = append(entries, e.ExportData(fields))
	}
	return map[string]any{
		"gitSHA":  d.GitSHA,
		"id":      d.ID,
		"entries": entries,
	}
}

// fetchTree resolves a directory listing for dirPath at ref using the REST Contents API.
func fetchTree(httpClient *http.Client, repo ghrepo.Interface, dirPath, ref string) (*repoDir, error) {
	client := api.NewClientFromHTTP(httpClient)

	cleanPath := strings.Trim(dirPath, "/")
	pathParts := []string{"repos", repo.RepoOwner(), repo.RepoName(), "contents"}
	if cleanPath != "" {
		pathParts = append(pathParts, strings.Split(cleanPath, "/")...)
	}
	u, err := safeurl.JoinPath(pathParts...)
	if err != nil {
		return nil, err
	}
	if ref != "" {
		u.SetQuery("ref", ref)
	}

	var raw json.RawMessage
	if err := client.REST(repo.RepoHost(), http.MethodGet, u.String(), nil, &raw); err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			if ref != "" {
				return nil, fmt.Errorf("could not find %q at %q in %s (the path or ref may not exist)", dirPath, ref, ghrepo.FullName(repo))
			}
			return nil, fmt.Errorf("could not find %q in %s", dirPath, ghrepo.FullName(repo))
		}
		return nil, err
	}

	type restContentEntry struct {
		Name            string `json:"name"`
		Path            string `json:"path"`
		Type            string `json:"type"`
		SHA             string `json:"sha"`
		Size            int    `json:"size"`
		SubmoduleGitURL string `json:"submodule_git_url"`
	}

	var items []restContentEntry
	if err := json.Unmarshal(raw, &items); err != nil {
		var single restContentEntry
		if objErr := json.Unmarshal(raw, &single); objErr == nil && single.Type != "" {
			if single.Type == "file" {
				return nil, fmt.Errorf("%q is a file, not a directory; use `gh repo read-file` instead", dirPath)
			}
			return nil, fmt.Errorf("%q is not a directory", dirPath)
		}
		return nil, err
	}

	dir := &repoDir{
		Entries: make([]dirEntry, 0, len(items)),
	}
	for _, item := range items {
		gitType := "blob"
		mode := modeFile | 0o644
		switch item.Type {
		case "dir":
			gitType = "tree"
			mode = modeDir
		case "symlink":
			gitType = "blob"
			mode = modeSymlink
		case "submodule":
			gitType = "commit"
			mode = modeSubmodule
		}

		entry := dirEntry{
			Name:    item.Name,
			Path:    item.Path,
			NameRaw: item.Name,
			PathRaw: item.Path,
			Type:    item.Type,
			GitType: gitType,
			Mode:    mode,
			GitSHA:  item.SHA,
			Size:    item.Size,
		}
		if item.Type == "submodule" && item.SubmoduleGitURL != "" {
			entry.Submodule = &submodule{
				GitURL:              item.SubmoduleGitURL,
				SubprojectCommitOid: item.SHA,
			}
		}
		dir.Entries = append(dir.Entries, entry)
	}

	return dir, nil
}

// entryTypeFromMode normalizes a git object mode into the entry type names the
// command reports: dir, file, symlink, or submodule. Modes with an unrecognized
// type are reported as "unknown".
func entryTypeFromMode(mode int) string {
	switch mode & modeTypeMask {
	case modeDir:
		return "dir"
	case modeFile:
		return "file"
	case modeSymlink:
		return "symlink"
	case modeSubmodule:
		return "submodule"
	default:
		return "unknown"
	}
}
