package copy

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

func TestNewCmdCopy(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         copyOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "not-a-number",
			cli:         "x --title t",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name: "title",
			cli:  "--title t",
			wants: copyOpts{
				title: "t",
			},
		},
		{
			name: "number",
			cli:  "123 --title t",
			wants: copyOpts{
				number: 123,
				title:  "t",
			},
		},
		{
			name: "source-owner",
			cli:  "--source-owner monalisa --title t",
			wants: copyOpts{
				sourceOwner: "monalisa",
				title:       "t",
			},
		},
		{
			name: "target-owner",
			cli:  "--target-owner monalisa --title t",
			wants: copyOpts{
				targetOwner: "monalisa",
				title:       "t",
			},
		},
		{
			name: "drafts",
			cli:  "--drafts --title t",
			wants: copyOpts{
				includeDraftIssues: true,
				title:              "t",
			},
		},
		{
			name: "json",
			cli:  "--format json --title t",
			wants: copyOpts{
				title: "t",
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

			var gotOpts copyOpts
			cmd := NewCmdCopy(f, func(config copyConfig) error {
				gotOpts = config.opts
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.sourceOwner, gotOpts.sourceOwner)
			assert.Equal(t, tt.wants.targetOwner, gotOpts.targetOwner)
			assert.Equal(t, tt.wants.title, gotOpts.title)
			assert.Equal(t, tt.wants.includeDraftIssues, gotOpts.includeDraftIssues)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunCopy_NotSupported(t *testing.T) {
	client := queries.NewTestClient()
	ios, _, _, _ := iostreams.Test()

	config := copyConfig{
		io: ios,
		opts: copyOpts{
			title:       "a title",
			sourceOwner: "monalisa",
			targetOwner: "monalisa",
			number:      1,
		},
		client: client,
	}

	err := runCopy(config)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
