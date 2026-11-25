package shared

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/gabriel-vasile/mimetype"
)

type GistFile struct {
	Filename  string `json:"filename,omitempty"`
	Type      string `json:"type,omitempty"`
	Language  string `json:"language,omitempty"`
	Content   string `json:"content"`
	RawURL    string `json:"raw_url,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type GistOwner struct {
	Login string `json:"login,omitempty"`
}

type Gist struct {
	ID          string               `json:"id,omitempty"`
	Description string               `json:"description"`
	Files       map[string]*GistFile `json:"files"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Public      bool                 `json:"public"`
	HTMLURL     string               `json:"html_url,omitempty"`
	Owner       *GistOwner           `json:"owner,omitempty"`
}

func (g Gist) Filename() string {
	filenames := make([]string, 0, len(g.Files))
	for fn := range g.Files {
		filenames = append(filenames, fn)
	}
	if len(filenames) == 0 {
		return ""
	}
	sort.Strings(filenames)
	return filenames[0]
}

func (g Gist) TruncDescription() string {
	return text.Truncate(100, text.RemoveExcessiveWhitespace(g.Description))
}

var NotFoundErr = errors.New("not found")

func GetGist(client *http.Client, hostname, gistID string) (*Gist, error) {
	gist := Gist{}
	path, err := safeurl.JoinPath("gists", gistID)
	if err != nil {
		return nil, err
	}

	apiClient := api.NewClientFromHTTP(client)
	err = apiClient.REST(hostname, "GET", path.String(), nil, &gist)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return nil, NotFoundErr
		}
		return nil, err
	}

	return &gist, nil
}

func GistIDFromURL(gistURL string) (string, error) {
	u, err := url.Parse(gistURL)
	if err == nil && strings.HasPrefix(u.Path, "/") {
		split := strings.Split(u.Path, "/")

		if len(split) > 2 {
			return split[2], nil
		}

		if len(split) == 2 && split[1] != "" {
			return split[1], nil
		}
	}

	return "", fmt.Errorf("Invalid gist URL %s", u)
}

const maxPerPage = 100

func ListGists(client *http.Client, hostname string, limit int, filter *regexp.Regexp, includeContent bool, visibility string) ([]Gist, error) {
	perPage := min(limit, maxPerPage)
	if perPage < 1 {
		perPage = 30
	}

	filterFunc := func(gist *Gist) bool {
		if filter.MatchString(gist.Description) {
			return true
		}

		for _, file := range gist.Files {
			if filter.MatchString(file.Filename) {
				return true
			}

			if includeContent && filter.MatchString(file.Content) {
				return true
			}
		}

		return false
	}

	u, err := safeurl.JoinPath("gists")
	if err != nil {
		return nil, err
	}
	u.SetQuery("per_page", strconv.Itoa(perPage))

	apiClient := api.NewClientFromHTTP(client)
	gists := []Gist{}
	vis := strings.ToLower(visibility)

	nextPath := u.String()
pagination:
	for nextPath != "" {
		var page []Gist
		next, err := apiClient.RESTWithNext(hostname, http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, err
		}

		for _, gist := range page {
			switch vis {
			case "public":
				if !gist.Public {
					continue
				}
			case "secret":
				if gist.Public {
					continue
				}
			}

			if gist.Files == nil {
				gist.Files = map[string]*GistFile{}
			}
			hasContent := false
			for name, f := range gist.Files {
				if f == nil {
					gist.Files[name] = &GistFile{Filename: name}
					continue
				}
				if f.Filename == "" {
					f.Filename = name
				}
				if f.Content != "" {
					hasContent = true
				}
			}

			if includeContent && !hasContent && len(gist.Files) > 0 {
				fullGist, err := GetGist(client, hostname, gist.ID)
				if err == nil && fullGist != nil {
					gist.Files = fullGist.Files
				}
			}

			if filter == nil || filterFunc(&gist) {
				gists = append(gists, gist)
			}

			if len(gists) == limit {
				break pagination
			}
		}

		nextPath = next
	}

	return gists, nil
}

func IsBinaryFile(file string) (bool, error) {
	detectedMime, err := mimetype.DetectFile(file)
	if err != nil {
		return false, err
	}

	isBinary := true
	for mime := detectedMime; mime != nil; mime = mime.Parent() {
		if mime.Is("text/plain") {
			isBinary = false
			break
		}
	}
	return isBinary, nil
}

func IsBinaryContents(contents []byte) bool {
	isBinary := true
	for mime := mimetype.Detect(contents); mime != nil; mime = mime.Parent() {
		if mime.Is("text/plain") {
			isBinary = false
			break
		}
	}
	return isBinary
}

func PromptGists(prompter prompter.Prompter, client *http.Client, host string, cs *iostreams.ColorScheme) (gist *Gist, err error) {
	gists, err := ListGists(client, host, 10, nil, false, "all")
	if err != nil {
		return &Gist{}, err
	}

	if len(gists) == 0 {
		return &Gist{}, nil
	}

	var opts = make([]string, len(gists))

	for i, gist := range gists {
		gistTime := text.FuzzyAgo(time.Now(), gist.UpdatedAt)
		// TODO: support dynamic maxWidth
		opts[i] = fmt.Sprintf("%s %s %s", cs.Bold(gist.Filename()), gist.TruncDescription(), cs.Muted(gistTime))
	}

	result, err := prompter.Select("Select a gist", "", opts)

	if err != nil {
		return &Gist{}, err
	}

	return &gists[result], nil
}

// GetRawGistFile fetches the full content of a gist file from its raw URL. The
// bytes are external content, so they are returned as iostreams.Untrusted to
// force callers to choose between sanitized display and raw round-tripping.
// The rawURL is supplied by the API and points at gist.githubusercontent.com rather than the
// API host, so it is requested as an absolute URL. The hostname only configures the client.
func GetRawGistFile(httpClient *http.Client, hostname string, rawURL safeurl.SafeURL) (iostreams.Untrusted, error) {
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(httpClient).Request(hostname, http.MethodGet, rawURL.String(), nil)
	if err != nil {
		return iostreams.Untrusted{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return iostreams.Untrusted{}, api.UnexpectedStatusError(resp)
	}

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return iostreams.Untrusted{}, err
	}

	return iostreams.NewUntrustedBytes(body), nil
}
