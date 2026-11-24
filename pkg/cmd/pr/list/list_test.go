package list

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func runCommand(rt http.RoundTripper, detector fd.Detector, isTTY bool, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	browser := &browser.Stub{}
	factory := &cmdutil.Factory{
		IOStreams: ios,
		Browser:   browser,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	fakeNow := func() time.Time {
		return time.Date(2022, time.August, 24, 23, 50, 0, 0, time.UTC)
	}

	cmd := NewCmdList(factory, func(opts *ListOptions) error {
		opts.Now = fakeNow
		opts.Detector = detector
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
		OutBuf:     stdout,
		ErrBuf:     stderr,
		BrowsedURL: browser.BrowsedURL(),
	}, err
}

func initFakeHTTP() *httpmock.Registry {
	return &httpmock.Registry{}
}

const restPRListJSON = `[
	{
		"number": 32,
		"title": "New feature",
		"state": "open",
		"draft": true,
		"created_at": "2022-08-24T20:01:12Z",
		"html_url": "https://github.com/OWNER/REPO/pull/32",
		"head": {"ref": "feature", "user": {"login": "OWNER"}},
		"base": {"ref": "main", "repo": {"full_name": "OWNER/REPO"}}
	},
	{
		"number": 29,
		"title": "Fixed bad bug",
		"state": "open",
		"draft": false,
		"created_at": "2022-07-20T19:01:12Z",
		"html_url": "https://github.com/OWNER/REPO/pull/29",
		"head": {"ref": "bug-fix", "user": {"login": "hubot"}, "repo": {"full_name": "hubot/REPO"}},
		"base": {"ref": "main", "repo": {"full_name": "OWNER/REPO"}}
	},
	{
		"number": 28,
		"title": "Improve documentation",
		"state": "closed",
		"merged_at": "2020-01-26T19:01:12Z",
		"created_at": "2020-01-26T19:01:12Z",
		"html_url": "https://github.com/OWNER/REPO/pull/28",
		"head": {"ref": "docs", "user": {"login": "OWNER"}},
		"base": {"ref": "main", "repo": {"full_name": "OWNER/REPO"}}
	}
]`

func TestPRList(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(httpmock.REST("GET", "repos/OWNER/REPO/pulls"), httpmock.StringResponse(restPRListJSON))

	output, err := runCommand(http, nil, true, "")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, heredoc.Doc(`

		Showing 3 of 3 open pull requests in OWNER/REPO

		ID   TITLE                  BRANCH         CREATED AT
		#32  New feature            feature        about 3 hours ago
		#29  Fixed bad bug          hubot:bug-fix  about 1 month ago
		#28  Improve documentation  docs           about 2 years ago
	`), output.String())
	assert.Equal(t, ``, output.Stderr())
}

func TestPRList_nontty(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(httpmock.REST("GET", "repos/OWNER/REPO/pulls"), httpmock.StringResponse(restPRListJSON))

	output, err := runCommand(http, nil, false, "")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "", output.Stderr())

	assert.Equal(t, `32	New feature	feature	DRAFT	2022-08-24T20:01:12Z
29	Fixed bad bug	hubot:bug-fix	OPEN	2022-07-20T19:01:12Z
28	Improve documentation	docs	MERGED	2020-01-26T19:01:12Z
`, output.String())
}

func TestPRList_filtering(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.QueryMatcher("GET", "repos/OWNER/REPO/pulls", url.Values{
			"per_page": {"30"},
			"state":    {"all"},
		}),
		httpmock.StatusStringResponse(500, ""))

	output, err := runCommand(http, nil, true, `-s all`)
	assert.Error(t, err)

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRList_filteringRemoveDuplicate(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[
			{"number": 32, "title": "New feature", "state": "open", "created_at": "2022-08-24T20:01:12Z", "head": {"ref": "feature", "user": {"login": "OWNER"}}},
			{"number": 32, "title": "New feature", "state": "open", "created_at": "2022-08-24T20:01:12Z", "head": {"ref": "feature", "user": {"login": "OWNER"}}}
		]`))

	output, err := runCommand(http, nil, true, "")
	if err != nil {
		t.Fatal(err)
	}

	out := output.String()
	idx := strings.Index(out, "New feature")
	if idx < 0 {
		t.Fatalf("text %q not found in %q", "New feature", out)
	}
	assert.Equal(t, idx, strings.LastIndex(out, "New feature"))
}

func TestPRList_filteringClosed(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.QueryMatcher("GET", "repos/OWNER/REPO/pulls", url.Values{
			"per_page": {"30"},
			"state":    {"closed"},
		}),
		httpmock.StatusStringResponse(500, ""))

	_, err := runCommand(http, nil, true, `-s closed`)
	assert.Error(t, err)
}

func TestPRList_filteringHeadBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.QueryMatcher("GET", "repos/OWNER/REPO/pulls", url.Values{
			"per_page": {"100"},
			"state":    {"open"},
			"head":     {"OWNER:bug-fix"},
		}),
		httpmock.StatusStringResponse(500, ""))

	_, err := runCommand(http, nil, true, `-H bug-fix`)
	assert.Error(t, err)
}

func TestPRList_filteringAssignee(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"per_page": {"30"},
			"q":        {`assignee:hubot base:develop is:merged label:"needs tests" repo:OWNER/REPO type:pr`},
		}),
		httpmock.StatusStringResponse(500, ""))

	_, err := runCommand(http, fd.AdvancedIssueSearchSupportedAsOptIn(), true, `-s merged -l "needs tests" -a hubot -B develop`)
	assert.Error(t, err)
}

func TestPRList_filteringDraft(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		expectedQuery string
	}{
		{
			name:          "draft",
			cli:           "--draft",
			expectedQuery: `draft:true repo:OWNER/REPO state:open type:pr`,
		},
		{
			name:          "non-draft",
			cli:           "--draft=false",
			expectedQuery: `draft:false repo:OWNER/REPO state:open type:pr`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			http := initFakeHTTP()
			defer http.Verify(t)

			http.Register(
				httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"per_page": {"30"},
					"q":        {test.expectedQuery},
				}),
				httpmock.StatusStringResponse(500, ""))

			_, err := runCommand(http, fd.AdvancedIssueSearchSupportedAsOptIn(), true, test.cli)
			assert.Error(t, err)
		})
	}
}

func TestPRList_filteringAuthor(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		expectedQuery string
	}{
		{
			name:          "author @me",
			cli:           `--author "@me"`,
			expectedQuery: `author:@me repo:OWNER/REPO state:open type:pr`,
		},
		{
			name:          "author user",
			cli:           `--author "monalisa"`,
			expectedQuery: `author:monalisa repo:OWNER/REPO state:open type:pr`,
		},
		{
			name:          "app author",
			cli:           `--author "app/dependabot"`,
			expectedQuery: `author:app/dependabot repo:OWNER/REPO state:open type:pr`,
		},
		{
			name:          "app author with app option",
			cli:           `--app "dependabot"`,
			expectedQuery: `author:app/dependabot repo:OWNER/REPO state:open type:pr`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			http := initFakeHTTP()
			defer http.Verify(t)

			http.Register(
				httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"per_page": {"30"},
					"q":        {test.expectedQuery},
				}),
				httpmock.StatusStringResponse(500, ""))

			_, err := runCommand(http, fd.AdvancedIssueSearchSupportedAsOptIn(), true, test.cli)
			assert.Error(t, err)
		})
	}
}

func TestPRList_withInvalidLimitFlag(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	_, err := runCommand(http, nil, true, `--limit=0`)
	assert.EqualError(t, err, "invalid value for --limit: 0")
}

func TestPRList_web(t *testing.T) {
	tests := []struct {
		name               string
		cli                string
		expectedBrowserURL string
	}{
		{
			name:               "filters",
			cli:                "-a peter -l bug -l docs -L 10 -s merged -B trunk",
			expectedBrowserURL: "https://github.com/OWNER/REPO/pulls?q=assignee%3Apeter+base%3Atrunk+is%3Amerged+label%3Abug+label%3Adocs+type%3Apr",
		},
		{
			name:               "draft",
			cli:                "--draft=true",
			expectedBrowserURL: "https://github.com/OWNER/REPO/pulls?q=draft%3Atrue+state%3Aopen+type%3Apr",
		},
		{
			name:               "non-draft",
			cli:                "--draft=0",
			expectedBrowserURL: "https://github.com/OWNER/REPO/pulls?q=draft%3Afalse+state%3Aopen+type%3Apr",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			http := initFakeHTTP()
			defer http.Verify(t)

			_, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			output, err := runCommand(http, nil, true, "--web "+test.cli)
			if err != nil {
				t.Errorf("error running command `pr list` with `--web` flag: %v", err)
			}

			assert.Equal(t, "", output.String())
			assert.Equal(t, "Opening https://github.com/OWNER/REPO/pulls in your browser.\n", output.Stderr())
			assert.Equal(t, test.expectedBrowserURL, output.BrowsedURL)
		})
	}
}

func TestPRList_unsupportedJSONFields(t *testing.T) {
	for _, field := range []string{"projectItems", "projectCards", "closingIssuesReferences", "reviewDecision"} {
		t.Run(field, func(t *testing.T) {
			http := initFakeHTTP()
			_, err := runCommand(http, nil, true, "--json "+field)
			assert.ErrorIs(t, err, api.ErrNotSupported)
		})
	}
}
