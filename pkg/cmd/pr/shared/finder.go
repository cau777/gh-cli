package shared

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	o "github.com/cli/cli/v2/pkg/option"
	"github.com/cli/cli/v2/pkg/set"
)

type PRFinder interface {
	Find(opts FindOptions) (*api.PullRequest, ghrepo.Interface, error)
}

type progressIndicator interface {
	StartProgressIndicator()
	StopProgressIndicator()
}

type GitConfigClient interface {
	ReadBranchConfig(ctx context.Context, branchName string) (git.BranchConfig, error)
	PushDefault(ctx context.Context) (git.PushDefault, error)
	RemotePushDefault(ctx context.Context) (string, error)
	PushRevision(ctx context.Context, branchName string) (git.RemoteTrackingRef, error)
}

type finder struct {
	baseRepoFn      func() (ghrepo.Interface, error)
	branchFn        func() (string, error)
	httpClient      func() (*http.Client, error)
	remotesFn       func() (ghContext.Remotes, error)
	gitConfigClient GitConfigClient
	progress        progressIndicator

	baseRefRepo ghrepo.Interface
	prNumber    int
	branchName  string
}

func NewFinder(factory *cmdutil.Factory) PRFinder {
	if finderForRunCommandStyleTests != nil {
		f := finderForRunCommandStyleTests
		finderForRunCommandStyleTests = &mockFinder{err: errors.New("you must use StubFinderForRunCommandStyleTests to stub PR lookups")}
		return f
	}

	return &finder{
		baseRepoFn:      factory.BaseRepo,
		branchFn:        factory.Branch,
		httpClient:      factory.HttpClient,
		gitConfigClient: factory.GitClient,
		remotesFn:       factory.Remotes,
		progress:        factory.IOStreams,
	}
}

var finderForRunCommandStyleTests PRFinder

// StubFinderForRunCommandStyleTests is the NewMockFinder substitute to be used ONLY in runCommand-style tests.
func StubFinderForRunCommandStyleTests(t *testing.T, selector string, pr *api.PullRequest, repo ghrepo.Interface) *mockFinder {
	// Create a new mock finder and override the "runCommandFinder" variable so that calls to
	// NewFinder() will return this mock. This is a bad pattern, and a result of old style runCommand
	// tests that would ideally be replaced. The reason we need to do this is that the runCommand style tests
	// construct the cobra command via NewCmd* functions, and then Execute them directly, providing no opportunity
	// to inject a test double unless it's on the factory, which finder never is, because only PR commands need it.
	finder := NewMockFinder(selector, pr, repo)
	finderForRunCommandStyleTests = finder

	// Ensure that at the end of the test, we reset the "runCommandFinder" variable so that tests are isolated,
	// at least if they are run sequentially.
	t.Cleanup(func() {
		finderForRunCommandStyleTests = nil
	})
	return finder
}

type FindOptions struct {
	// Selector can be a number with optional `#` prefix, a branch name with optional `<owner>:` prefix, or
	// a PR URL.
	Selector string
	// Fields lists the GraphQL fields to fetch for the PullRequest.
	Fields []string
	// BaseBranch is the name of the base branch to scope the PR-for-branch lookup to.
	BaseBranch string
	// States lists the possible PR states to scope the PR-for-branch lookup to.
	States []string

	DisableProgress bool

	Detector fd.Detector
}

func (f *finder) Find(opts FindOptions) (*api.PullRequest, ghrepo.Interface, error) {
	// If we have a URL, we don't need git stuff
	if len(opts.Fields) == 0 {
		return nil, nil, errors.New("Find error: no fields specified")
	}

	if repo, prNumber, _, err := ParseURL(opts.Selector); err == nil {
		f.prNumber = prNumber
		f.baseRefRepo = repo
	}

	if f.baseRefRepo == nil {
		repo, err := f.baseRepoFn()
		if err != nil {
			return nil, nil, err
		}
		f.baseRefRepo = repo
	}

	var prRefs PRFindRefs
	if opts.Selector == "" {
		// You must be in a git repo for this case to work
		currentBranchName, err := f.branchFn()
		if err != nil {
			return nil, nil, err
		}
		f.branchName = currentBranchName

		// Get the branch config for the current branchName
		branchConfig, err := f.gitConfigClient.ReadBranchConfig(context.Background(), f.branchName)
		if err != nil {
			return nil, nil, err
		}

		// Determine if the branch is configured to merge to a special PR ref
		prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
		if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
			prNumber, _ := strconv.Atoi(m[1])
			f.prNumber = prNumber
		}

		// Determine the PullRequestRefs from config
		if f.prNumber == 0 {
			prRefsResolver := NewPullRequestFindRefsResolver(
				// We requested the branch config already, so let's cache that
				CachedBranchConfigGitConfigClient{
					CachedBranchConfig: branchConfig,
					GitConfigClient:    f.gitConfigClient,
				},
				f.remotesFn,
			)
			prRefs, err = prRefsResolver.ResolvePullRequestRefs(f.baseRefRepo, opts.BaseBranch, f.branchName)
			if err != nil {
				return nil, nil, err
			}
		}
	} else if f.prNumber == 0 {
		// You gave me a selector but I couldn't find a PR number (it wasn't a URL)

		// Try to get a PR number from the selector
		prNumber, err := strconv.Atoi(strings.TrimPrefix(opts.Selector, "#"))
		// If opts.Selector is a valid number then assume it is the
		// PR number unless opts.BaseBranch is specified. This is a
		// special case for PR create command which will always want
		// to assume that a numerical selector is a branch name rather
		// than PR number.
		if opts.BaseBranch == "" && err == nil {
			f.prNumber = prNumber
		} else {
			f.branchName = opts.Selector

			qualifiedHeadRef, err := ParseQualifiedHeadRef(f.branchName)
			if err != nil {
				return nil, nil, err
			}

			prRefs = PRFindRefs{
				qualifiedHeadRef: qualifiedHeadRef,
				baseRepo:         f.baseRefRepo,
				baseBranchName:   o.SomeIfNonZero(opts.BaseBranch),
			}
		}
	}

	// Set up HTTP client
	httpClient, err := f.httpClient()
	if err != nil {
		return nil, nil, err
	}

	// TODO: Decouple the PR finder from IO
	// TODO(josebalius): Should we be guarding here?
	if !opts.DisableProgress && f.progress != nil {
		f.progress.StartProgressIndicator()
		defer f.progress.StopProgressIndicator()
	}

	fields := set.NewStringSet()
	fields.AddValues(opts.Fields)
	fields.AddValues([]string{"id", "number"})

	// Projects V1/V2, closingIssuesReferences, mergeQueue, and reviewDecision are not
	// available via REST; leave them empty during Finder lookups and validate explicit
	// --json requests at the command layer.
	fields.Remove("projectItems")
	fields.Remove("projectCards")
	fields.Remove("closingIssuesReferences")
	fields.Remove("isInMergeQueue")
	fields.Remove("isMergeQueueEnabled")

	var pr *api.PullRequest
	if f.prNumber > 0 {
		pr, err = findByNumber(httpClient, f.baseRefRepo, f.prNumber, fields.ToSlice())
		if err != nil {
			return pr, f.baseRefRepo, err
		}
	} else if prRefs.BaseRepo() != nil && f.branchName != "" {
		// No PR number, but we have a base repo and branch name.
		pr, err = findForRefs(httpClient, prRefs, opts.States, fields.ToSlice())
		if err != nil {
			return pr, f.baseRefRepo, err
		}
	} else {
		// If we don't have a PR number or a base repo and branch name,
		// we can't do anything
		return nil, f.baseRefRepo, &NotFoundError{fmt.Errorf("no pull requests found")}
	}

	return pr, f.baseRefRepo, nil
}

var pullURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/pull/(\d+)(.*$)`)

// ParseURL parses a pull request URL and returns the repository, pull request
// number, and any tailing path components. If there is no error, the returned
// repo is not nil and will have non-empty hostname.
func ParseURL(prURL string) (ghrepo.Interface, int, string, error) {
	if prURL == "" {
		return nil, 0, "", fmt.Errorf("invalid URL: %q", prURL)
	}

	u, err := url.Parse(prURL)
	if err != nil {
		return nil, 0, "", err
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, 0, "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	m := pullURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, 0, "", fmt.Errorf("not a pull request URL: %s", prURL)
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	prNumber, _ := strconv.Atoi(m[3])
	tail := m[4]
	return repo, prNumber, tail, nil
}

var fullReferenceRE = regexp.MustCompile(`^(?:([^/]+)/([^/]+))#(\d+)$`)

// ParseFullReference parses a short issue/pull request reference of the form
// "owner/repo#number", where owner, repo and number are all required.
func ParseFullReference(s string) (ghrepo.Interface, int, error) {
	if s == "" {
		return nil, 0, errors.New("empty reference")
	}

	m := fullReferenceRE.FindStringSubmatch(s)
	if m == nil {
		return nil, 0, fmt.Errorf("invalid reference: %q", s)
	}

	number, err := strconv.Atoi(m[3])
	if err != nil {
		return nil, 0, fmt.Errorf("invalid reference: %q; %w", s, err)
	}

	owner := m[1]
	repo := m[2]

	return ghrepo.New(owner, repo), number, nil
}

func findByNumber(httpClient *http.Client, repo ghrepo.Interface, number int, fields []string) (*api.PullRequest, error) {
	client := api.NewClientFromHTTP(httpClient)
	return api.LookupPullRequestByNumberREST(client, repo, number, fields)
}

func findForRefs(httpClient *http.Client, prRefs PRFindRefs, stateFilters, fields []string) (*api.PullRequest, error) {
	headQuery := prRefs.QualifiedHeadRef()
	if !strings.Contains(headQuery, ":") {
		headQuery = fmt.Sprintf("%s:%s", prRefs.BaseRepo().RepoOwner(), prRefs.UnqualifiedHeadRef())
	}

	client := api.NewClientFromHTTP(httpClient)
	prs, defaultBranch, err := api.FetchPullRequestsForBranchREST(client, prRefs.BaseRepo(), headQuery, stateFilters)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(prs, func(a, b int) bool {
		return prs[a].State == "OPEN" && prs[b].State != "OPEN"
	})

	for i := range prs {
		pr := &prs[i]
		// When the head is the default branch, it doesn't really make sense to show merged or closed PRs.
		// https://github.com/cli/cli/issues/4263
		isNotClosedOrMergedWhenHeadIsDefault := pr.State == "OPEN" || defaultBranch != prRefs.QualifiedHeadRef()
		if prRefs.Matches(pr.BaseRefName, pr.HeadLabel()) && isNotClosedOrMergedWhenHeadIsDefault {
			if api.NeedsSinglePRDetailFetch(fields) {
				fullPR, err := api.FetchPullRequestREST(client, prRefs.BaseRepo(), pr.Number)
				if err != nil {
					return nil, err
				}
				pr = fullPR
			}
			if err := api.HydratePullRequestFields(client, prRefs.BaseRepo(), pr, fields); err != nil {
				return nil, err
			}
			return pr, nil
		}
	}

	return nil, &NotFoundError{fmt.Errorf("no pull requests found for branch %q", prRefs.QualifiedHeadRef())}
}

type NotFoundError struct {
	error
}

func (err *NotFoundError) Unwrap() error {
	return err.error
}

func NewMockFinder(selector string, pr *api.PullRequest, repo ghrepo.Interface) *mockFinder {
	var err error
	if pr == nil {
		err = &NotFoundError{errors.New("no pull requests found")}
	}
	return &mockFinder{
		expectSelector: selector,
		pr:             pr,
		repo:           repo,
		err:            err,
	}
}

type mockFinder struct {
	called         bool
	expectSelector string
	expectFields   []string
	pr             *api.PullRequest
	repo           ghrepo.Interface
	err            error
}

func (m *mockFinder) Find(opts FindOptions) (*api.PullRequest, ghrepo.Interface, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	if m.expectSelector != opts.Selector {
		return nil, nil, fmt.Errorf("mockFinder: expected selector %q, got %q", m.expectSelector, opts.Selector)
	}
	if len(m.expectFields) > 0 && !isEqualSet(m.expectFields, opts.Fields) {
		return nil, nil, fmt.Errorf("mockFinder: expected fields %v, got %v", m.expectFields, opts.Fields)
	}
	if m.called {
		return nil, nil, errors.New("mockFinder used more than once")
	}
	m.called = true

	if m.pr.HeadRepositoryOwner.Login == "" {
		// pose as same-repo PR by default
		m.pr.HeadRepositoryOwner.Login = m.repo.RepoOwner()
	}

	return m.pr, m.repo, nil
}

func (m *mockFinder) ExpectFields(fields []string) {
	m.expectFields = fields
}

func isEqualSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	aCopy := make([]string, len(a))
	copy(aCopy, a)
	bCopy := make([]string, len(b))
	copy(bCopy, b)
	sort.Strings(aCopy)
	sort.Strings(bCopy)

	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}
