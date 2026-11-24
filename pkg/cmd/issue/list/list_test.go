package list

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wantsErr bool
		wants    ListOptions
	}{
		{
			name: "type flag",
			cli:  "--type Bug",
			wants: ListOptions{
				IssueType:    "Bug",
				State:        "open",
				LimitResults: 30,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}
			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
				gotOpts = opts
				return nil
			})
			argv, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, gotOpts)

			assert.Equal(t, tt.wants.IssueType, gotOpts.IssueType)
			assert.Equal(t, tt.wants.State, gotOpts.State)
			assert.Equal(t, tt.wants.LimitResults, gotOpts.LimitResults)
		})
	}
}

func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewMockConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	fakeNow := func() time.Time {
		return time.Date(2022, time.August, 25, 23, 50, 0, 0, time.UTC)
	}

	cmd := NewCmdList(factory, func(opts *ListOptions) error {
		opts.Now = fakeNow
		return listRun(opts)
	})

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

const restIssueListFixture = `[
	{"number": 1, "title": "number won", "html_url": "https://wow.com", "updated_at": "2022-08-24T22:01:12Z", "labels": [{"name": "label"}]},
	{"number": 2, "title": "number too", "html_url": "https://wow.com", "updated_at": "2022-07-20T19:01:12Z", "labels": [{"name": "label"}]},
	{"number": 4, "title": "number fore", "html_url": "https://wow.com", "updated_at": "2020-01-26T19:01:12Z", "labels": [{"name": "label"}]}
]`

func TestIssueList_nontty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues"),
		httpmock.StringResponse(restIssueListFixture))

	output, err := runCommand(http, false, "")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	assert.Equal(t, "", output.Stderr())
	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.String(),
		`1[\t]+number won[\t]+label[\t]+\d+`,
		`2[\t]+number too[\t]+label[\t]+\d+`,
		`4[\t]+number fore[\t]+label[\t]+\d+`)
}

func TestIssueList_tty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues"),
		httpmock.StringResponse(restIssueListFixture))

	output, err := runCommand(http, true, "")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	assert.Equal(t, heredoc.Doc(`

		Showing 3 of 3 open issues in OWNER/REPO

		ID  TITLE        LABELS  UPDATED
		#1  number won   label   about 1 day ago
		#2  number too   label   about 1 month ago
		#4  number fore  label   about 2 years ago
	`), output.String())
	assert.Equal(t, ``, output.Stderr())
}

func TestIssueList_tty_withFlags(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.QueryMatcher("GET", "repos/OWNER/REPO/issues", url.Values{
			"assignee":  {"probablyCher"},
			"creator":   {"foo"},
			"mentioned": {"me"},
			"state":     {"open"},
			"per_page":  {"100"},
		}),
		httpmock.StringResponse(`[]`))

	output, err := runCommand(http, true, "-a probablyCher -s open -A foo --mention me")
	assert.EqualError(t, err, "no issues match your search in OWNER/REPO")

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestIssueList_tty_withAppFlag(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.QueryMatcher("GET", "repos/OWNER/REPO/issues", url.Values{
			"creator":  {"app/dependabot"},
			"state":    {"open"},
			"per_page": {"100"},
		}),
		httpmock.StringResponse(`[]`))

	output, err := runCommand(http, true, "--app dependabot")
	assert.EqualError(t, err, "no issues match your search in OWNER/REPO")

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestIssueList_withInvalidLimitFlag(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	_, err := runCommand(http, true, "--limit=0")

	if err == nil || err.Error() != "invalid limit: 0" {
		t.Errorf("error running command `issue list`: %v", err)
	}
}

func TestIssueList_disabledIssues(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues"),
		httpmock.StatusStringResponse(410, `{"message": "Issues are disabled for this repo"}`),
	)

	_, err := runCommand(http, true, "")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue list`: %v", err)
	}
}

// TODO advancedIssueSearchCleanup
// Simplify this test to only a single test case once GHES 3.17 support ends.
func TestIssueList_web(t *testing.T) {
	tests := []struct {
		name     string
		detector fd.Detector
	}{
		{
			name:     "advanced issue search not supported",
			detector: fd.AdvancedIssueSearchUnsupported(),
		},
		{
			name:     "advanced issue search supported as opt-in",
			detector: fd.AdvancedIssueSearchSupportedAsOptIn(),
		},
		{
			name:     "advanced issue search supported as only backend",
			detector: fd.AdvancedIssueSearchSupportedAsOnlyBackend(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStderrTTY(true)
			browser := &browser.Stub{}

			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			_, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			opts := &ListOptions{
				IO:      ios,
				Browser: browser,
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				Detector:     tt.detector,
				WebMode:      true,
				State:        "all",
				Assignee:     "peter",
				Author:       "john",
				Labels:       []string{"bug", "docs"},
				Mention:      "frank",
				Milestone:    "v1.1",
				LimitResults: 10,
			}

			err := listRun(opts)
			require.NoError(t, err)

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "Opening https://github.com/OWNER/REPO/issues in your browser.\n", stderr.String())

			// Since no repeated usage of special search qualifiers is possible
			// with our current implementation, we can assert against the same
			// URL for both search backend (i.e. legacy and advanced issue search).
			browser.Verify(t, "https://github.com/OWNER/REPO/issues?q=assignee%3Apeter+author%3Ajohn+label%3Abug+label%3Adocs+mentions%3Afrank+milestone%3Av1.1+type%3Aissue")
		})
	}
}

func Test_issueList(t *testing.T) {
	type args struct {
		repo    ghrepo.Interface
		filters prShared.FilterOptions
		limit   int
	}
	tests := []struct {
		name      string
		args      args
		httpStubs func(*httpmock.Registry)
		wantErr   bool
	}{
		{
			name: "default",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity: "issue",
					State:  "open",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/issues", url.Values{
						"per_page": {"100"},
						"state":    {"open"},
					}),
					httpmock.StringResponse(`[]`))
			},
		},
		{
			name: "milestone by number",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity:    "issue",
					State:     "open",
					Milestone: "13",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/milestones/13"),
					httpmock.StringResponse(`{"title": "1.x", "number": 13}`))
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", url.Values{
						"per_page": {"30"},
						"q":        {"milestone:1.x repo:OWNER/REPO state:open type:issue"},
					}),
					httpmock.StringResponse(`{"total_count": 0, "items": []}`))
			},
		},
		{
			name: "milestone by title",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity:    "issue",
					State:     "open",
					Milestone: "1.x",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", url.Values{
						"per_page": {"30"},
						"q":        {"milestone:1.x repo:OWNER/REPO state:open type:issue"},
					}),
					httpmock.StringResponse(`{"total_count": 0, "items": []}`))
			},
		},
		{
			name: "@me syntax",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity:   "issue",
					State:    "open",
					Author:   "@me",
					Assignee: "@me",
					Mention:  "@me",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"monalisa"}`))
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/issues", url.Values{
						"per_page":  {"100"},
						"state":     {"open"},
						"assignee":  {"monalisa"},
						"creator":   {"monalisa"},
						"mentioned": {"monalisa"},
					}),
					httpmock.StringResponse(`[]`))
			},
		},
		{
			name: "@me with search",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity:   "issue",
					State:    "open",
					Author:   "@me",
					Assignee: "@me",
					Mention:  "@me",
					Search:   "auth bug",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", url.Values{
						"per_page": {"30"},
						"q":        {"( auth bug ) assignee:@me author:@me mentions:@me repo:OWNER/REPO state:open type:issue"},
					}),
					httpmock.StringResponse(`{"total_count": 0, "items": []}`))
			},
		},
		{
			name: "with labels",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity: "issue",
					State:  "open",
					Labels: []string{"hello", "one world"},
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", url.Values{
						"per_page": {"30"},
						"q":        {`label:"one world" label:hello repo:OWNER/REPO state:open type:issue`},
					}),
					httpmock.StringResponse(`{"total_count": 0, "items": []}`))
			},
		},
		{
			name: "with issue type",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity:    "issue",
					State:     "open",
					IssueType: "Bug",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", url.Values{
						"per_page": {"30"},
						"q":        {"repo:OWNER/REPO state:open type:Bug type:issue"},
					}),
					httpmock.StringResponse(`{"total_count": 0, "items": []}`))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpreg := &httpmock.Registry{}
			defer httpreg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(httpreg)
			}
			client := &http.Client{Transport: httpreg}
			_, err := issueList(client, tt.args.repo, tt.args.filters, tt.args.limit)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIssueList_json_unsupportedFields(t *testing.T) {
	for _, field := range []string{"projectItems", "projectCards", "closedByPullRequestsReferences", "isPinned"} {
		t.Run(field, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			_, err := runCommand(reg, false, "--json "+field)
			require.Error(t, err)
			assert.ErrorIs(t, err, api.ErrNotSupported)
		})
	}
}
