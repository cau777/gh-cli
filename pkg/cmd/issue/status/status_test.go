package status

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
)

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

	cmd := NewCmdStatus(factory, nil)

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

func TestIssueStatus(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("GET", "user"),
		httpmock.StringResponse(`{"login":"octocat"}`))
	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO"),
		httpmock.StringResponse(`{"has_issues": true}`))
	http.Register(
		httpmock.QueryMatcher("GET", "search/issues", map[string][]string{
			"q":        {"repo:OWNER/REPO is:issue is:open assignee:octocat"},
			"sort":     {"updated"},
			"order":    {"desc"},
			"per_page": {"10"},
		}),
		httpmock.StringResponse(`{"total_count":1,"items":[{"number":8,"title":"carrots","state":"open","updated_at":"2020-01-01T00:00:00Z"}]}`))
	http.Register(
		httpmock.QueryMatcher("GET", "search/issues", map[string][]string{
			"q":        {"repo:OWNER/REPO is:issue is:open mentions:octocat"},
			"sort":     {"updated"},
			"order":    {"desc"},
			"per_page": {"10"},
		}),
		httpmock.StringResponse(`{"total_count":1,"items":[{"number":9,"title":"squash","state":"open","updated_at":"2020-01-01T00:00:00Z"}]}`))
	http.Register(
		httpmock.QueryMatcher("GET", "search/issues", map[string][]string{
			"q":        {"repo:OWNER/REPO is:issue is:open author:octocat"},
			"sort":     {"updated"},
			"order":    {"desc"},
			"per_page": {"10"},
		}),
		httpmock.StringResponse(`{"total_count":2,"items":[{"number":10,"title":"broccoli","state":"open","updated_at":"2020-01-01T00:00:00Z"},{"number":11,"title":"swiss chard","state":"open","updated_at":"2020-01-01T00:00:00Z"}]}`))

	output, err := runCommand(http, true, "")
	if err != nil {
		t.Errorf("error running command `issue status`: %v", err)
	}

	expectedIssues := []*regexp.Regexp{
		regexp.MustCompile(`(?m)8.*carrots.*about.*ago`),
		regexp.MustCompile(`(?m)9.*squash.*about.*ago`),
		regexp.MustCompile(`(?m)10.*broccoli.*about.*ago`),
		regexp.MustCompile(`(?m)11.*swiss chard.*about.*ago`),
	}

	for _, r := range expectedIssues {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueStatus_blankSlate(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("GET", "user"),
		httpmock.StringResponse(`{"login":"octocat"}`))
	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO"),
		httpmock.StringResponse(`{"has_issues": true}`))
	http.Register(
		httpmock.REST("GET", "search/issues"),
		httpmock.StringResponse(`{"total_count":0,"items":[]}`))
	http.Register(
		httpmock.REST("GET", "search/issues"),
		httpmock.StringResponse(`{"total_count":0,"items":[]}`))
	http.Register(
		httpmock.REST("GET", "search/issues"),
		httpmock.StringResponse(`{"total_count":0,"items":[]}`))

	output, err := runCommand(http, true, "")
	if err != nil {
		t.Errorf("error running command `issue status`: %v", err)
	}

	expectedOutput := `
Relevant issues in OWNER/REPO

Issues assigned to you
  There are no issues assigned to you

Issues mentioning you
  There are no issues mentioning you

Issues opened by you
  There are no issues opened by you

`
	if output.String() != expectedOutput {
		t.Errorf("expected %q, got %q", expectedOutput, output)
	}
}

func TestIssueStatus_disabledIssues(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)
	http.Exclude(t, httpmock.REST("GET", "search/issues"))

	http.Register(
		httpmock.REST("GET", "user"),
		httpmock.StringResponse(`{"login":"octocat"}`))
	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO"),
		httpmock.StringResponse(`{"has_issues": false}`))

	_, err := runCommand(http, true, "")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue status`: %v", err)
	}
}
