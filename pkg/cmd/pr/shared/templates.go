package shared

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/githubtemplate"
	"gopkg.in/yaml.v3"
)

type issueTemplate struct {
	Gname  string
	Gbody  string
	Gtitle string
}

type pullRequestTemplate struct {
	Gname string
	Gbody string
}

func (t *issueTemplate) Name() string {
	return t.Gname
}

func (t *issueTemplate) NameForSubmit() string {
	return t.Gname
}

func (t *issueTemplate) Body() []byte {
	return []byte(t.Gbody)
}

func (t *issueTemplate) Title() string {
	return t.Gtitle
}

func (t *pullRequestTemplate) Name() string {
	return t.Gname
}

func (t *pullRequestTemplate) NameForSubmit() string {
	return ""
}

func (t *pullRequestTemplate) Body() []byte {
	return []byte(t.Gbody)
}

func (t *pullRequestTemplate) Title() string {
	return ""
}

type restTemplateEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

var templateFrontmatterRE = regexp.MustCompile(`(?m)^---\r?\n(\s*\r?\n)?`)

func isHTTPNotFoundError(err error) bool {
	var httpErr api.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

func fetchRESTContentFile(client *api.Client, repo ghrepo.Interface, filePath string) ([]byte, error) {
	parts := append([]string{"repos", repo.RepoOwner(), repo.RepoName(), "contents"}, strings.Split(filePath, "/")...)
	u, err := safeurl.JoinPath(parts...)
	if err != nil {
		return nil, err
	}

	var file restTemplateEntry
	if err := client.REST(repo.RepoHost(), http.MethodGet, u.String(), nil, &file); err != nil {
		return nil, err
	}

	if file.Encoding == "" || file.Encoding != "base64" {
		return []byte(file.Content), nil
	}
	return base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
}

func listRESTTemplateEntries(client *api.Client, repo ghrepo.Interface, dirName string) ([]restTemplateEntry, error) {
	candidates := [][]string{
		{".github", dirName},
		{dirName},
		{"docs", dirName},
	}

	for _, parts := range candidates {
		urlParts := append([]string{"repos", repo.RepoOwner(), repo.RepoName(), "contents"}, parts...)
		u, err := safeurl.JoinPath(urlParts...)
		if err != nil {
			return nil, err
		}

		var entries []restTemplateEntry
		err = client.REST(repo.RepoHost(), http.MethodGet, u.String(), nil, &entries)
		if err != nil {
			if isHTTPNotFoundError(err) {
				continue
			}
			return nil, err
		}
		return entries, nil
	}

	return nil, nil
}

func listIssueTemplates(httpClient *http.Client, repo ghrepo.Interface) ([]Template, error) {
	client := api.NewClientFromHTTP(httpClient)
	entries, err := listRESTTemplateEntries(client, repo, "ISSUE_TEMPLATE")
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	var templates []Template
	for _, entry := range entries {
		if entry.Type != "" && entry.Type != "file" {
			continue
		}
		ext := strings.ToLower(path.Ext(entry.Name))
		if ext != ".md" && ext != ".yml" && ext != ".yaml" {
			continue
		}
		if ext != ".md" && strings.EqualFold(entry.Name, "config.yml") || strings.EqualFold(entry.Name, "config.yaml") {
			continue
		}

		raw, err := fetchRESTContentFile(client, repo, entry.Path)
		if err != nil {
			return nil, err
		}

		tpl := parseRESTIssueTemplate(entry.Name, raw)
		templates = append(templates, &tpl)
	}

	return templates, nil
}

func parseRESTIssueTemplate(filename string, raw []byte) issueTemplate {
	tpl := issueTemplate{Gname: filename}
	ext := strings.ToLower(path.Ext(filename))
	if ext == ".yml" || ext == ".yaml" {
		var meta struct {
			Name  string
			Title string
		}
		if err := yaml.Unmarshal(raw, &meta); err == nil {
			if meta.Name != "" {
				tpl.Gname = meta.Name
			}
			tpl.Gtitle = meta.Title
		}
		return tpl
	}

	if matches := templateFrontmatterRE.FindAllIndex(raw, 2); len(matches) > 1 && matches[0][0] == 0 {
		var meta struct {
			Name  string
			Title string
		}
		if err := yaml.Unmarshal(raw[0:matches[1][1]], &meta); err == nil {
			if meta.Name != "" {
				tpl.Gname = meta.Name
			}
			tpl.Gtitle = meta.Title
		}
		tpl.Gbody = string(raw[matches[1][1]:])
	} else {
		tpl.Gbody = string(raw)
	}
	return tpl
}

func listPullRequestTemplates(httpClient *http.Client, repo ghrepo.Interface) ([]Template, error) {
	client := api.NewClientFromHTTP(httpClient)
	entries, err := listRESTTemplateEntries(client, repo, "PULL_REQUEST_TEMPLATE")
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	var templates []Template
	for _, entry := range entries {
		if entry.Type != "" && entry.Type != "file" {
			continue
		}
		if !strings.EqualFold(path.Ext(entry.Name), ".md") {
			continue
		}

		raw, err := fetchRESTContentFile(client, repo, entry.Path)
		if err != nil {
			return nil, err
		}

		templates = append(templates, &pullRequestTemplate{
			Gname: entry.Name,
			Gbody: string(raw),
		})
	}

	return templates, nil
}

type Template interface {
	Name() string
	NameForSubmit() string
	Body() []byte
	Title() string
}

type iprompter interface {
	Select(string, string, []string) (int, error)
}

type templateManager struct {
	repo       ghrepo.Interface
	rootDir    string
	allowFS    bool
	isPR       bool
	httpClient *http.Client
	detector   fd.Detector
	prompter   iprompter

	templates      []Template
	legacyTemplate Template

	didFetch   bool
	fetchError error
}

func NewTemplateManager(httpClient *http.Client, repo ghrepo.Interface, p iprompter, dir string, allowFS bool, isPR bool) *templateManager {
	cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
	return &templateManager{
		repo:       repo,
		rootDir:    dir,
		allowFS:    allowFS,
		isPR:       isPR,
		httpClient: httpClient,
		prompter:   p,
		detector:   fd.NewDetector(cachedClient, repo.RepoHost()),
	}
}

func (m *templateManager) hasAPI() (bool, error) {
	if !m.isPR {
		return true, nil
	}

	features, err := m.detector.RepositoryFeatures()
	if err != nil {
		return false, err
	}

	return features.PullRequestTemplateQuery, nil
}

func (m *templateManager) HasTemplates() (bool, error) {
	if err := m.memoizedFetch(); err != nil {
		return false, err
	}
	return len(m.templates) > 0, nil
}

func (m *templateManager) LegacyBody() []byte {
	if m.legacyTemplate == nil {
		return nil
	}
	return m.legacyTemplate.Body()
}

func (m *templateManager) Choose() (Template, error) {
	if err := m.memoizedFetch(); err != nil {
		return nil, err
	}
	if len(m.templates) == 0 {
		return nil, nil
	}

	names := make([]string, len(m.templates))
	for i, t := range m.templates {
		names[i] = t.Name()
	}

	blankOption := "Open a blank issue"
	if m.isPR {
		blankOption = "Open a blank pull request"
	}

	selectedOption, err := m.prompter.Select("Choose a template", "", append(names, blankOption))
	if err != nil {
		return nil, fmt.Errorf("could not prompt: %w", err)
	}

	if selectedOption == len(names) {
		return nil, nil
	}
	return m.templates[selectedOption], nil
}

func (m *templateManager) Select(name string) (Template, error) {
	if err := m.memoizedFetch(); err != nil {
		return nil, err
	}

	if len(m.templates) == 0 {
		return nil, errors.New("no templates found")
	}

	for _, t := range m.templates {
		if t.Name() == name {
			return t, nil
		}
	}

	return nil, fmt.Errorf("template %q not found", name)
}

func (m *templateManager) memoizedFetch() error {
	if m.didFetch {
		return m.fetchError
	}
	m.fetchError = m.fetch()
	m.didFetch = true
	return m.fetchError
}

func (m *templateManager) fetch() error {
	hasAPI, err := m.hasAPI()
	if err != nil {
		return err
	}

	if hasAPI {
		lister := listIssueTemplates
		if m.isPR {
			lister = listPullRequestTemplates
		}
		templates, err := lister(m.httpClient, m.repo)
		if err != nil {
			return err
		}
		m.templates = templates
	}

	if !m.allowFS {
		return nil
	}

	dir := m.rootDir
	if dir == "" {
		var err error
		gitClient := &git.Client{}
		dir, err = gitClient.ToplevelDir(context.Background())
		if err != nil {
			//nolint:nilerr // intentional, abort silently
			return nil
		}
	}

	filePattern := "ISSUE_TEMPLATE"
	if m.isPR {
		filePattern = "PULL_REQUEST_TEMPLATE"
	}

	if !hasAPI {
		issueTemplates := githubtemplate.FindNonLegacy(dir, filePattern)
		m.templates = make([]Template, len(issueTemplates))
		for i, t := range issueTemplates {
			m.templates[i] = &filesystemTemplate{path: t}
		}
	}

	if legacyTemplate := githubtemplate.FindLegacy(dir, filePattern); legacyTemplate != "" {
		m.legacyTemplate = &filesystemTemplate{path: legacyTemplate}
	}

	return nil
}

type filesystemTemplate struct {
	path string
}

func (t *filesystemTemplate) Name() string {
	return githubtemplate.ExtractName(t.path)
}

func (t *filesystemTemplate) NameForSubmit() string {
	return ""
}

func (t *filesystemTemplate) Body() []byte {
	return githubtemplate.ExtractContents(t.path)
}

func (t *filesystemTemplate) Title() string {
	return githubtemplate.ExtractTitle(t.path)
}
