package update_branch

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	shared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdUpdateBranch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		output   UpdateBranchOptions
		wantsErr string
	}{
		{
			name:   "no argument",
			input:  "",
			output: UpdateBranchOptions{},
		},
		{
			name:  "with argument",
			input: "23",
			output: UpdateBranchOptions{
				SelectorArg: "23",
			},
		},
		{
			name:  "no argument, --rebase",
			input: "--rebase",
			output: UpdateBranchOptions{
				Rebase: true,
			},
		},
		{
			name:  "with argument, --rebase",
			input: "23 --rebase",
			output: UpdateBranchOptions{
				SelectorArg: "23",
				Rebase:      true,
			},
		},
		{
			name:     "no argument, --repo",
			input:    "--repo owner/repo",
			wantsErr: "argument required when using the --repo flag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var gotOpts *UpdateBranchOptions
			cmd := NewCmdUpdateBranch(f, func(opts *UpdateBranchOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.PersistentFlags().StringP("repo", "R", "", "")

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr != "" {
				assert.EqualError(t, err, tt.wantsErr)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.SelectorArg, gotOpts.SelectorArg)
			assert.Equal(t, tt.output.Rebase, gotOpts.Rebase)
		})
	}
}

func Test_updateBranchRun(t *testing.T) {
	defaultInput := func() UpdateBranchOptions {
		return UpdateBranchOptions{
			Finder: shared.NewMockFinder("123", &api.PullRequest{
				ID:                  "123",
				Number:              123,
				BaseRefName:         "base-ref-name",
				HeadRefOid:          "head-ref-oid",
				HeadRefName:         "head-ref-name",
				HeadRepositoryOwner: api.Owner{Login: "head-repository-owner"},
				Mergeable:           api.PullRequestMergeableMergeable,
			}, ghrepo.New("OWNER", "REPO")),
		}
	}

	tests := []struct {
		name      string
		input     *UpdateBranchOptions
		httpStubs func(*testing.T, *httpmock.Registry)
		stdout    string
		stderr    string
		wantsErr  string
	}{
		{
			name: "failure, pr not found",
			input: &UpdateBranchOptions{
				Finder:      shared.NewMockFinder("", nil, nil),
				SelectorArg: "123",
			},
			wantsErr: "no pull requests found",
		},
		{
			name: "success, already up-to-date",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/compare/base-ref-name...head-repository-owner:head-ref-name"),
					httpmock.StringResponse(`{"ahead_by": 999, "behind_by": 0, "status": "ahead"}`))
			},
			stdout: "✓ PR branch already up-to-date\n",
		},
		{
			name: "success, already up-to-date, PR branch on the same repo as base",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					ID:                  "123",
					Number:              123,
					BaseRefName:         "base-ref-name",
					HeadRefOid:          "head-ref-oid",
					HeadRefName:         "head-ref-name",
					HeadRepositoryOwner: api.Owner{Login: "OWNER"},
					Mergeable:           api.PullRequestMergeableMergeable,
				}, ghrepo.New("OWNER", "REPO")),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/compare/base-ref-name...head-ref-name"),
					httpmock.StringResponse(`{"ahead_by": 999, "behind_by": 0, "status": "ahead"}`))
			},
			stdout: "✓ PR branch already up-to-date\n",
		},
		{
			name: "failure, not mergeable due to conflicts",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					ID:                  "123",
					Number:              123,
					BaseRefName:         "base-ref-name",
					HeadRefOid:          "head-ref-oid",
					HeadRefName:         "head-ref-name",
					HeadRepositoryOwner: api.Owner{Login: "OWNER"},
					Mergeable:           api.PullRequestMergeableConflicting,
				}, ghrepo.New("OWNER", "REPO")),
			},
			stderr:   "X Cannot update PR branch due to conflicts\n",
			wantsErr: cmdutil.SilentError.Error(),
		},
		{
			name: "success, merge",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/compare/base-ref-name...head-repository-owner:head-ref-name"),
					httpmock.StringResponse(`{"ahead_by": 0, "behind_by": 999, "status": "behind"}`))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/pulls/123/update-branch"),
					httpmock.RESTPayload(202, `{}`, func(payload map[string]any) {
						assert.Equal(t, map[string]any{"expected_head_sha": "head-ref-oid"}, payload)
					}))
			},
			stdout: "✓ PR branch updated\n",
		},
		{
			name: "failure, rebase not supported",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
				Rebase:      true,
			},
			wantsErr: "not all features are supported: rebasing a pull request branch (--rebase) is not supported with REST",
		},
		{
			name: "failure, API error on ref comparison request",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/compare/base-ref-name...head-repository-owner:head-ref-name"),
					httpmock.StatusJSONResponse(500, struct{ Message string }{Message: "some error"}))
			},
			wantsErr: "HTTP 500: some error (https://api.github.com/repos/OWNER/REPO/compare/base-ref-name...head-repository-owner:head-ref-name)",
		},
		{
			name: "failure, merge conflict error on update request",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/compare/base-ref-name...head-repository-owner:head-ref-name"),
					httpmock.StringResponse(`{"ahead_by": 999, "behind_by": 999, "status": "behind"}`))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/pulls/123/update-branch"),
					httpmock.StatusJSONResponse(422, struct{ Message string }{Message: "merge conflict between base and head"}))
			},
			stderr:   "X Cannot update PR branch due to conflicts\n",
			wantsErr: cmdutil.SilentError.Error(),
		},
		{
			name: "failure, API error on update request",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/compare/base-ref-name...head-repository-owner:head-ref-name"),
					httpmock.StringResponse(`{"ahead_by": 0, "behind_by": 999, "status": "behind"}`))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/pulls/123/update-branch"),
					httpmock.StatusJSONResponse(500, struct{ Message string }{Message: "some error"}))
			},
			wantsErr: "HTTP 500: some error (https://api.github.com/repos/OWNER/REPO/pulls/123/update-branch)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			tt.input.GitClient = &git.Client{
				GhPath:  "some/path/gh",
				GitPath: "some/path/git",
			}

			if tt.input.Finder == nil {
				tt.input.Finder = defaultInput().Finder
			}

			httpClient := func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }

			tt.input.IO = ios
			tt.input.HttpClient = httpClient

			err := updateBranchRun(tt.input)

			if tt.wantsErr != "" {
				assert.EqualError(t, err, tt.wantsErr)
				return
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())
		})
	}
}
