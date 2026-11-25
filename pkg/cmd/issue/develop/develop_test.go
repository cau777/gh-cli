package develop

import (
	"bytes"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/argparsetest"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDevelop(t *testing.T) {
	// Test shared parsing of issue number / URL.
	argparsetest.TestArgParsing(t, NewCmdDevelop)

	absWorktree, err := filepath.Abs("work trees/issue-1")
	require.NoError(t, err)

	tests := []struct {
		name             string
		input            string
		output           DevelopOptions
		expectedBaseRepo ghrepo.Interface
		wantStdout       string
		wantStderr       string
		wantErr          bool
		errMsg           string
	}{
		{
			name:  "branch-repo flag",
			input: "1 --branch-repo owner/repo",
			output: DevelopOptions{
				IssueNumber: 1,
				BranchRepo:  "owner/repo",
			},
		},
		{
			name:  "base flag",
			input: "1 --base feature",
			output: DevelopOptions{
				IssueNumber: 1,
				BaseBranch:  "feature",
			},
		},
		{
			name:  "checkout flag",
			input: "1 --checkout",
			output: DevelopOptions{
				IssueNumber: 1,
				Checkout:    true,
			},
		},
		{
			name:  "worktree flag",
			input: `1 --checkout --worktree "work trees/issue-1"`,
			output: DevelopOptions{
				IssueNumber: 1,
				Checkout:    true,
				Worktree:    absWorktree,
			},
		},
		{
			name:  "list flag",
			input: "1 --list",
			output: DevelopOptions{
				IssueNumber: 1,
				List:        true,
			},
		},
		{
			name:  "name flag",
			input: "1 --name feature",
			output: DevelopOptions{
				IssueNumber: 1,
				Name:        "feature",
			},
		},
		{
			name:  "issue-repo flag",
			input: "1 --issue-repo cli/cli",
			output: DevelopOptions{
				IssueNumber: 1,
			},
			wantStdout: "Flag --issue-repo has been deprecated, use `--repo` instead\n",
		},
		{
			name:    "list and branch repo flags",
			input:   "1 --list --branch-repo owner/repo",
			wantErr: true,
			errMsg:  "specify only one of `--list` or `--branch-repo`",
		},
		{
			name:    "list and base flags",
			input:   "1 --list --base feature",
			wantErr: true,
			errMsg:  "specify only one of `--list` or `--base`",
		},
		{
			name:    "list and checkout flags",
			input:   "1 --list --checkout",
			wantErr: true,
			errMsg:  "specify only one of `--list` or `--checkout`",
		},
		{
			name:    "list and name flags",
			input:   "1 --list --name my-branch",
			wantErr: true,
			errMsg:  "specify only one of `--list` or `--name`",
		},
		{
			name:    "blank worktree flag",
			input:   `1 --checkout --worktree ""`,
			wantErr: true,
			errMsg:  "--worktree cannot be blank",
		},
		{
			name:    "worktree without checkout",
			input:   "1 --worktree worktree",
			wantErr: true,
			errMsg:  "--worktree requires --checkout",
		},
		{
			name:    "list and worktree flags",
			input:   "1 --list --worktree worktree",
			wantErr: true,
			errMsg:  "specify only one of `--list` or `--worktree`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdOut, stdErr := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *DevelopOptions
			cmd := NewCmdDevelop(f, func(opts *DevelopOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(stdOut)
			cmd.SetErr(stdErr)

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				require.EqualError(t, err, tt.errMsg)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.output.IssueNumber, gotOpts.IssueNumber)
			assert.Equal(t, tt.output.Name, gotOpts.Name)
			assert.Equal(t, tt.output.BaseBranch, gotOpts.BaseBranch)
			assert.Equal(t, tt.output.Checkout, gotOpts.Checkout)
			assert.Equal(t, tt.output.List, gotOpts.List)
			assert.Equal(t, tt.output.Worktree, gotOpts.Worktree)
			assert.Equal(t, tt.wantStdout, stdOut.String())
			assert.Equal(t, tt.wantStderr, stdErr.String())
			if tt.expectedBaseRepo != nil {
				baseRepo, err := gotOpts.BaseRepo()
				require.NoError(t, err)
				require.True(
					t,
					ghrepo.IsSame(tt.expectedBaseRepo, baseRepo),
					"expected base repo %+v, got %+v", tt.expectedBaseRepo, baseRepo,
				)
			}
		})
	}
}

func TestDevelopRun(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	ios, _, _, _ := iostreams.Test()
	opts := &DevelopOptions{
		IssueNumber: 42,
		List:        true,
		IO:          ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	err := developRun(opts)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
