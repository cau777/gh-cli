package unlink

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func TestNewCmdUnlink(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         unlinkOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "not-a-number",
			cli:         "x",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name:        "specify-repo-and-team",
			cli:         "--repo my-repo --team my-team",
			wantsErr:    true,
			wantsErrMsg: "specify only one of `--repo` or `--team`",
		},
		{
			name: "specify-nothing",
			cli:  "",
			wants: unlinkOpts{
				repo:  "REPO",
				owner: "OWNER",
			},
		},
		{
			name: "repo",
			cli:  "--repo my-repo",
			wants: unlinkOpts{
				repo: "my-repo",
			},
		},
		{
			name: "repo-flag-contains-owner",
			cli:  "--repo monalisa/my-repo",
			wants: unlinkOpts{
				owner: "monalisa",
				repo:  "my-repo",
			},
		},
		{
			name: "repo-flag-contains-owner-and-host",
			cli:  "--repo github.com/monalisa/my-repo",
			wants: unlinkOpts{
				host:  "github.com",
				owner: "monalisa",
				repo:  "my-repo",
			},
		},
		{
			name:        "repo-flag-contains-wrong-format",
			cli:         "--repo h/e/l/l/o",
			wantsErr:    true,
			wantsErrMsg: "expected the \"[HOST/]OWNER/REPO\" or \"REPO\" format, got \"h/e/l/l/o\"",
		},
		{
			name:        "repo-flag-with-owner-different-from-owner-flag",
			cli:         "--repo monalisa/my-repo --owner leonardo",
			wantsErr:    true,
			wantsErrMsg: "'monalisa/my-repo' has different owner from 'leonardo'",
		},
		{
			name: "team",
			cli:  "--team my-team",
			wants: unlinkOpts{
				team: "my-team",
			},
		},
		{
			name: "team-flag-contains-owner",
			cli:  "--team my-org/my-team",
			wants: unlinkOpts{
				owner: "my-org",
				team:  "my-team",
			},
		},
		{
			name: "team-flag-contains-owner-and-host",
			cli:  "--team github.com/my-org/my-team",
			wants: unlinkOpts{
				host:  "github.com",
				owner: "my-org",
				team:  "my-team",
			},
		},
		{
			name:        "team-flag-contains-wrong-format",
			cli:         "--team h/e/l/l/o",
			wantsErr:    true,
			wantsErrMsg: "expected the \"[HOST/]OWNER/TEAM\" or \"TEAM\" format, got \"h/e/l/l/o\"",
		},
		{
			name:        "team-flag-with-owner-different-from-owner-flag",
			cli:         "--team my-org/my-team --owner her-org",
			wantsErr:    true,
			wantsErrMsg: "'my-org/my-team' has different owner from 'her-org'",
		},
		{
			name: "number",
			cli:  "123 --repo my-repo",
			wants: unlinkOpts{
				number: 123,
				repo:   "my-repo",
			},
		},
		{
			name: "owner-with-repo-flag",
			cli:  "--repo my-repo --owner monalisa",
			wants: unlinkOpts{
				owner: "monalisa",
				repo:  "my-repo",
			},
		},
		{
			name: "owner-without-repo-flag",
			cli:  "--owner monalisa",
			wants: unlinkOpts{
				owner: "monalisa",
				repo:  "REPO",
			},
		},
	}

	t.Setenv("GH_TOKEN", "auth-token")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			}

			argv, err := shlex.Split(tt.cli)
			require.NoError(t, err)

			var gotOpts unlinkOpts
			cmd := NewCmdUnlink(f, func(config unlinkConfig) error {
				gotOpts = config.opts
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				require.Error(t, err)
				require.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			require.NoError(t, err)

			require.Equal(t, tt.wants.number, gotOpts.number)
			require.Equal(t, tt.wants.owner, gotOpts.owner)
			require.Equal(t, tt.wants.repo, gotOpts.repo)
			require.Equal(t, tt.wants.team, gotOpts.team)
			require.Equal(t, tt.wants.projectID, gotOpts.projectID)
			require.Equal(t, tt.wants.repoID, gotOpts.repoID)
			require.Equal(t, tt.wants.teamID, gotOpts.teamID)
		})
	}
}

func TestRunUnlink_NotSupported(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	cfg := unlinkConfig{
		opts: unlinkOpts{
			number: 1,
			repo:   "my-repo",
			owner:  "monalisa",
		},
		client: queries.NewTestClient(),
		httpClient: func() (*http.Client, error) {
			return http.DefaultClient, nil
		},
		config: func() (gh.Config, error) {
			return config.NewMockConfig(), nil
		},
		io: ios,
	}

	err := runUnlink(cfg)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
