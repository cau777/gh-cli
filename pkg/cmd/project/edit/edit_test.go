package edit

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

func TestNewCmdEdit(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         editOpts
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
			name:        "visibility-error",
			cli:         "--visibility v",
			wantsErr:    true,
			wantsErrMsg: "invalid argument \"v\" for \"--visibility\" flag: valid values are {PUBLIC|PRIVATE}",
		},
		{
			name:        "no-args",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "no fields to edit",
		},
		{
			name: "title",
			cli:  "--title t",
			wants: editOpts{
				title: "t",
			},
		},
		{
			name: "number",
			cli:  "123 --title t",
			wants: editOpts{
				number: 123,
				title:  "t",
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa --title t",
			wants: editOpts{
				owner: "monalisa",
				title: "t",
			},
		},
		{
			name: "readme",
			cli:  "--readme r",
			wants: editOpts{
				readme: "r",
			},
		},
		{
			name: "description",
			cli:  "--description d",
			wants: editOpts{
				shortDescription: "d",
			},
		},
		{
			name: "visibility",
			cli:  "--visibility PUBLIC",
			wants: editOpts{
				visibility: "PUBLIC",
			},
		},
		{
			name: "json",
			cli:  "--format json --title t",
			wants: editOpts{
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

			var gotOpts editOpts
			cmd := NewCmdEdit(f, func(config editConfig) error {
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
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wants.visibility, gotOpts.visibility)
			assert.Equal(t, tt.wants.title, gotOpts.title)
			assert.Equal(t, tt.wants.readme, gotOpts.readme)
			assert.Equal(t, tt.wants.shortDescription, gotOpts.shortDescription)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunEdit_NotSupported(t *testing.T) {
	client := queries.NewTestClient()
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := editConfig{
		opts: editOpts{
			number:           1,
			owner:            "monalisa",
			title:            "a new title",
			shortDescription: "a new description",
			visibility:       "PUBLIC",
			readme:           "a new readme",
		},
		client: client,
		io:     ios,
	}

	err := runEdit(config)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
