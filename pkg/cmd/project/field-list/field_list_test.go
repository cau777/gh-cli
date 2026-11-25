package fieldlist

import (
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         listOpts
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
			name: "number",
			cli:  "123",
			wants: listOpts{
				number: 123,
				limit:  30,
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa",
			wants: listOpts{
				owner: "monalisa",
				limit: 30,
			},
		},
		{
			name: "json",
			cli:  "--format json",
			wants: listOpts{
				limit: 30,
			},
			wantsExporter: true,
		},
	}

	t.Setenv("GH_TOKEN", "auth-token")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts listOpts
			cmd := NewCmdList(f, func(config listConfig) error {
				gotOpts = config.opts
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wants.limit, gotOpts.limit)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunList_NotSupported(t *testing.T) {
	client := queries.NewTestClient()
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := listConfig{
		opts: listOpts{
			number: 1,
			owner:  "monalisa",
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
