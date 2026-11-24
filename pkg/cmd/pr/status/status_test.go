package status

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func runCommand(rt http.RoundTripper, branch string, isTTY bool, cli string) (*test.CmdOut, error) {
	return runCommandWithDetector(rt, branch, isTTY, cli, &fd.DisabledDetectorMock{})
}

func runCommandWithDetector(rt http.RoundTripper, branch string, isTTY bool, cli string, detector fd.Detector) (*test.CmdOut, error) {
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
		Remotes: func() (context.Remotes, error) {
			return context.Remotes{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   ghrepo.New("OWNER", "REPO"),
				},
			}, nil
		},
		Branch: func() (string, error) {
			if branch == "" {
				return "", git.ErrNotOnAnyBranch
			}
			return branch, nil
		},
		GitClient: &git.Client{GitPath: "some/path/git"},
	}

	withProvidedDetector := func(opts *StatusOptions) error {
		opts.Detector = detector
		return statusRun(opts)
	}

	cmd := NewCmdStatus(factory, withProvidedDetector)
	cmd.PersistentFlags().StringP("repo", "R", "", "")

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

func initFakeHTTP() *httpmock.Registry {
	return &httpmock.Registry{}
}

func TestPRStatus(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)

	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"per_page": {"10"},
			"q":        {"repo:OWNER/REPO state:open is:pr author:@me"},
		}),
		httpmock.StringResponse(`{"total_count":1,"items":[{"number":8,"title":"Strawberries are not actually berries","state":"open","html_url":"https://github.com/cli/cli/pull/8"}]}`),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"per_page": {"10"},
			"q":        {"repo:OWNER/REPO state:open is:pr review-requested:@me"},
		}),
		httpmock.StringResponse(`{"total_count":2,"items":[{"number":9,"title":"Apples are tasty","state":"open","html_url":"https://github.com/cli/cli/pull/9"},{"number":11,"title":"Figs are my favorite","state":"open","draft":true,"html_url":"https://github.com/cli/cli/pull/1"}]}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[{"number":10,"title":"Blueberries are a good fruit","state":"open","html_url":"https://github.com/cli/cli/pull/10","head":{"ref":"blueberries","sha":"sha10","user":{"login":"OWNER"}},"base":{"ref":"main","repo":{"full_name":"OWNER/REPO","default_branch":"main"}}}]`),
	)
	stubStatusPRDetails(reg, 8, `{"number":8,"title":"Strawberries are not actually berries","state":"open","html_url":"https://github.com/cli/cli/pull/8","head":{"ref":"strawberries","sha":"sha8","user":{"login":"OWNER"}}}`, `[]`, `[]`, `[]`)
	stubStatusPRDetails(reg, 9, `{"number":9,"title":"Apples are tasty","state":"open","html_url":"https://github.com/cli/cli/pull/9","head":{"ref":"apples","sha":"sha9","user":{"login":"OWNER"}},"auto_merge":{"merge_method":"squash","enabled_by":{"login":"hubot"}}}`, nil, `[]`, `[]`)
	stubStatusPRDetails(reg, 10, `{"number":10,"title":"Blueberries are a good fruit","state":"open","html_url":"https://github.com/cli/cli/pull/10","head":{"ref":"blueberries","sha":"sha10","user":{"login":"OWNER"}},"base":{"repo":{"full_name":"OWNER/REPO"}}}`, `[]`, `[]`, `[]`)
	stubStatusPRDetails(reg, 11, `{"number":11,"title":"Figs are my favorite","state":"open","draft":true,"html_url":"https://github.com/cli/cli/pull/1","head":{"ref":"figs","sha":"sha11","user":{"login":"OWNER"}}}`, nil, `[]`, `[]`)

	// stub successful git commands
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommand(reg, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedPrs := []*regexp.Regexp{
		regexp.MustCompile(`#8.*\[strawberries\]`),
		regexp.MustCompile(`#9.*\[apples\].*✓ Auto-merge enabled`),
		regexp.MustCompile(`#10.*\[blueberries\]`),
		regexp.MustCompile(`#11.*\[figs\]`),
	}

	for _, r := range expectedPrs {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/", r)
		}
	}
}

func TestPRStatus_reviewsAndChecks(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)

	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"per_page": {"10"},
			"q":        {"repo:OWNER/REPO state:open is:pr author:@me"},
		}),
		httpmock.StringResponse(`{"total_count":4,"items":[
			{"number":8,"title":"Strawberries are not actually berries","state":"open"},
			{"number":7,"title":"Bananas are berries","state":"open"},
			{"number":6,"title":"Avocado is probably not a berry","state":"open"},
			{"number":5,"title":"Why can't berries get along?","state":"open"}
		]}`),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"per_page": {"10"},
			"q":        {"repo:OWNER/REPO state:open is:pr review-requested:@me"},
		}),
		httpmock.StringResponse(`{"total_count":0,"items":[]}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[]`),
	)

	stubStatusPRDetails(reg, 8, `{"number":8,"title":"Strawberries are not actually berries","state":"open","html_url":"https://github.com/cli/cli/pull/8","mergeable":null,"head":{"ref":"strawberries","sha":"sha8","user":{"login":"OWNER"}}}`,
		`[{"user":{"login":"reviewer"},"state":"CHANGES_REQUESTED","submitted_at":"2020-01-01T00:00:00Z"}]`,
		`[]`, `[{"state":"success"}]`)
	stubStatusPRDetails(reg, 7, `{"number":7,"title":"Bananas are berries","state":"open","html_url":"https://github.com/cli/cli/pull/7","mergeable":true,"head":{"ref":"banananana","sha":"sha7","user":{"login":"OWNER"}}}`,
		`[{"user":{"login":"bob"},"state":"APPROVED","submitted_at":"2020-01-01T00:00:00Z"},{"user":{"login":"alice"},"state":"APPROVED","submitted_at":"2020-01-01T00:00:00Z"}]`,
		`[{"status":"in_progress","conclusion":""}]`, `[]`)
	stubStatusPRDetails(reg, 6, `{"number":6,"title":"Avocado is probably not a berry","state":"open","html_url":"https://github.com/cli/cli/pull/6","mergeable":true,"head":{"ref":"avo","sha":"sha6","user":{"login":"OWNER"}}}`,
		`[]`,
		`[{"status":"in_progress","conclusion":""},{"status":"completed","conclusion":"neutral"}]`, `[{"state":"failure"}]`)
	stubStatusPRDetails(reg, 5, `{"number":5,"title":"Why can't berries get along?","state":"open","html_url":"https://github.com/cli/cli/pull/5","mergeable":false,"head":{"ref":"strawberries","sha":"sha5","user":{"login":"OWNER"}}}`,
		`[]`, `[]`, `[{"state":"success"}]`)

	// stub successful git command
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommand(reg, "blueberries", true, "-c")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := []string{
		"✓ Checks passing + Changes requested ! Merge conflict status unknown",
		"- Checks pending ✓ 2 Approved ✓ No merge conflicts",
		"× 1/3 checks failing ✓ No merge conflicts",
		"✓ Checks passing × Merge conflicts",
	}

	for _, line := range expected {
		if !strings.Contains(output.String(), line) {
			t.Errorf("output did not contain %q: %q", line, output.String())
		}
	}
}

func TestPRStatus_reviewsAndChecksWithStatesByCount(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)

	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"per_page": {"10"},
			"q":        {"repo:OWNER/REPO state:open is:pr author:@me"},
		}),
		httpmock.StringResponse(`{"total_count":2,"items":[
			{"number":8,"title":"Strawberries are not actually berries","state":"open"},
			{"number":7,"title":"Bananas are berries","state":"open"}
		]}`),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"per_page": {"10"},
			"q":        {"repo:OWNER/REPO state:open is:pr review-requested:@me"},
		}),
		httpmock.StringResponse(`{"total_count":0,"items":[]}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[]`),
	)

	stubStatusPRDetails(reg, 8, `{"number":8,"title":"Strawberries are not actually berries","state":"open","mergeable":null,"head":{"ref":"strawberries","sha":"sha8","user":{"login":"OWNER"}}}`,
		`[{"user":{"login":"reviewer"},"state":"CHANGES_REQUESTED"}]`,
		`[]`, `[{"state":"success"}]`)
	stubStatusPRDetails(reg, 7, `{"number":7,"title":"Bananas are berries","state":"open","mergeable":false,"head":{"ref":"banananana","sha":"sha7","user":{"login":"OWNER"}}}`,
		`[{"user":{"login":"bob"},"state":"APPROVED"},{"user":{"login":"alice"},"state":"APPROVED"}]`,
		`[{"status":"in_progress","conclusion":""}]`, `[]`)

	// stub successful git command
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommandWithDetector(reg, "blueberries", true, "-c", &fd.EnabledDetectorMock{})
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := []string{
		"✓ Checks passing + Changes requested ! Merge conflict status unknown",
		"- Checks pending ✓ 2 Approved × Merge conflicts",
	}

	for _, line := range expected {
		if !strings.Contains(output.String(), line) {
			t.Errorf("output did not contain %q: %q", line, output.String())
		}
	}
}

func TestPRStatus_currentBranch_showTheMostRecentPR(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[
			{"number":10,"title":"Blueberries are certainly a good fruit","state":"open","head":{"ref":"blueberries","sha":"sha10","user":{"login":"OWNER"}},"base":{"repo":{"default_branch":"main"}}},
			{"number":9,"title":"Blueberries are a good fruit","state":"closed","merged_at":"2020-01-01T00:00:00Z","head":{"ref":"blueberries","sha":"sha9","user":{"login":"OWNER"}},"base":{"repo":{"default_branch":"main"}}},
			{"number":8,"title":"Blueberries are probably a good fruit","state":"closed","head":{"ref":"blueberries","sha":"sha8","user":{"login":"OWNER"}},"base":{"repo":{"default_branch":"main"}}}
		]`))
	stubStatusPRDetails(reg, 10, `{"number":10,"title":"Blueberries are certainly a good fruit","state":"open","head":{"ref":"blueberries","sha":"sha10","user":{"login":"OWNER"}}}`, `[]`, `[]`, `[]`)

	// stub successful git command
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommand(reg, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#10  Blueberries are certainly a good fruit \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}

	unexpectedLines := []*regexp.Regexp{
		regexp.MustCompile(`#9  Blueberries are a good fruit \[blueberries\] - Merged`),
		regexp.MustCompile(`#8  Blueberries are probably a good fruit \[blueberries\] - Closed`),
	}
	for _, r := range unexpectedLines {
		if r.MatchString(output.String()) {
			t.Errorf("output unexpectedly match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestPRStatus_currentBranch_defaultBranch(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[{"number":10,"title":"Blueberries are certainly a good fruit","state":"open","head":{"ref":"blueberries","sha":"sha10","user":{"login":"OWNER"}},"base":{"repo":{"default_branch":"blueberries"}}}]`))
	stubStatusPRDetails(reg, 10, `{"number":10,"title":"Blueberries are certainly a good fruit","state":"open","head":{"ref":"blueberries","sha":"sha10","user":{"login":"OWNER"}}}`, `[]`, `[]`, `[]`)

	// stub successful git command
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommand(reg, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#10  Blueberries are certainly a good fruit \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_defaultBranch_repoFlag(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)

	output, err := runCommand(reg, "blueberries", true, "-R OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\]`)
	if expectedLine.MatchString(output.String()) {
		t.Errorf("output not expected to match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Closed(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[{"number":8,"title":"Blueberries are a good fruit","state":"closed","head":{"ref":"blueberries","sha":"sha8","user":{"login":"OWNER"}},"base":{"repo":{"default_branch":"main"}}}]`))
	stubStatusPRDetails(reg, 8, `{"number":8,"title":"Blueberries are a good fruit","state":"closed","head":{"ref":"blueberries","sha":"sha8","user":{"login":"OWNER"}}}`, `[]`, `[]`, `[]`)

	// stub successful git command
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommand(reg, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\] - Closed`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Closed_defaultBranch(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[{"number":8,"title":"Blueberries are a good fruit","state":"closed","head":{"ref":"blueberries","sha":"sha8","user":{"login":"OWNER"}},"base":{"repo":{"default_branch":"blueberries"}}}]`))
	stubStatusPRDetails(reg, 8, `{"number":8,"title":"Blueberries are a good fruit","state":"closed","head":{"ref":"blueberries","sha":"sha8","user":{"login":"OWNER"}}}`, `[]`, `[]`, `[]`)

	// stub successful git command
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommand(reg, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`There is no pull request associated with \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Merged(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[{"number":8,"title":"Blueberries are a good fruit","state":"closed","merged_at":"2020-01-01T00:00:00Z","head":{"ref":"blueberries","sha":"sha8","user":{"login":"OWNER"}},"base":{"repo":{"default_branch":"main"}}}]`))
	stubStatusPRDetails(reg, 8, `{"number":8,"title":"Blueberries are a good fruit","state":"closed","merged":true,"merged_at":"2020-01-01T00:00:00Z","head":{"ref":"blueberries","sha":"sha8","user":{"login":"OWNER"}}}`, `[]`, `[]`, `[]`)

	// stub successful git command
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommand(reg, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\] - Merged`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Merged_defaultBranch(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/pulls"),
		httpmock.StringResponse(`[{"number":8,"title":"Blueberries are a good fruit","state":"closed","merged_at":"2020-01-01T00:00:00Z","head":{"ref":"blueberries","sha":"sha8","user":{"login":"OWNER"}},"base":{"repo":{"default_branch":"blueberries"}}}]`))
	stubStatusPRDetails(reg, 8, `{"number":8,"title":"Blueberries are a good fruit","state":"closed","merged":true,"merged_at":"2020-01-01T00:00:00Z","head":{"ref":"blueberries","sha":"sha8","user":{"login":"OWNER"}}}`, `[]`, `[]`, `[]`)

	// stub successful git command
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommand(reg, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`There is no pull request associated with \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_blankSlate(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)
	reg.Register(httpmock.REST("GET", "repos/OWNER/REPO/pulls"), httpmock.StringResponse(`[]`))

	// stub successful git command
	rs, cleanup := run.Stub()
	defer cleanup(t)
	rs.Register(`git config --get-regexp \^branch\\.`, 0, "")
	rs.Register(`git config remote.pushDefault`, 0, "")
	rs.Register(`git rev-parse --symbolic-full-name blueberries@{push}`, 128, "")
	rs.Register(`git config push.default`, 1, "")

	output, err := runCommand(reg, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := `
Relevant pull requests in OWNER/REPO

Current branch
  There is no pull request associated with [blueberries]

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`
	if output.String() != expected {
		t.Errorf("expected %q, got %q", expected, output.String())
	}
}

func TestPRStatus_blankSlateRepoOverride(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)

	output, err := runCommand(reg, "blueberries", true, "--repo OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := `
Relevant pull requests in OWNER/REPO

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`
	if output.String() != expected {
		t.Errorf("expected %q, got %q", expected, output.String())
	}
}

func TestPRStatus_detachedHead(t *testing.T) {
	reg := initFakeHTTP()
	defer reg.Verify(t)
	stubStatusEmptySearch(reg)

	output, err := runCommand(reg, "", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := `
Relevant pull requests in OWNER/REPO

Current branch
  There is no current branch

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`
	if output.String() != expected {
		t.Errorf("expected %q, got %q", expected, output.String())
	}
}

func TestPRStatus_unsupportedJSONFields(t *testing.T) {
	for _, field := range []string{"projectItems", "projectCards", "closingIssuesReferences", "reviewDecision"} {
		t.Run(field, func(t *testing.T) {
			reg := initFakeHTTP()
			_, err := runCommand(reg, "blueberries", true, "--json "+field)
			assert.ErrorIs(t, err, api.ErrNotSupported)
		})
	}
}

func stubStatusEmptySearch(reg *httpmock.Registry) {
	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"per_page": {"10"},
			"q":        {"repo:OWNER/REPO state:open is:pr author:@me"},
		}),
		httpmock.StringResponse(`{"total_count":0,"items":[]}`),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{
			"per_page": {"10"},
			"q":        {"repo:OWNER/REPO state:open is:pr review-requested:@me"},
		}),
		httpmock.StringResponse(`{"total_count":0,"items":[]}`),
	)
}

func stubStatusPRDetails(reg *httpmock.Registry, number int, prJSON string, reviewsJSON any, checkRunsJSON, statusesJSON string) {
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/OWNER/REPO/pulls/%d", number)),
		httpmock.StringResponse(prJSON),
	)
	if reviewsJSON != nil {
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("repos/OWNER/REPO/pulls/%d/reviews", number)),
			httpmock.StringResponse(reviewsJSON.(string)),
		)
	}
	sha := fmt.Sprintf("sha%d", number)
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/OWNER/REPO/commits/%s/check-runs", sha)),
		httpmock.StringResponse(`{"check_runs":`+checkRunsJSON+`}`),
	)
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/OWNER/REPO/commits/%s/status", sha)),
		httpmock.StringResponse(`{"statuses":`+statusesJSON+`}`),
	)
}

func TestPRStatus_error_ReadBranchConfig(t *testing.T) {
	rs, cleanup := run.Stub()
	defer cleanup(t)
	// We only need the one stub because this fails early
	rs.Register(`git config --get-regexp \^branch\\.`, 2, "")
	_, err := runCommand(initFakeHTTP(), "blueberries", true, "")
	assert.Error(t, err)
}
