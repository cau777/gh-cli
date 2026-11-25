package itemcreate

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

func TestNewCmdCreateItem(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         createItemOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "missing-title",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"title\" not set",
		},
		{
			name:        "not-a-number",
			cli:         "x --title t",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name: "title",
			cli:  "--title t",
			wants: createItemOpts{
				title: "t",
			},
		},
		{
			name: "number",
			cli:  "123  --title t",
			wants: createItemOpts{
				number: 123,
				title:  "t",
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa --title t",
			wants: createItemOpts{
				owner: "monalisa",
				title: "t",
			},
		},
		{
			name: "body",
			cli:  "--body b --title t",
			wants: createItemOpts{
				body:  "b",
				title: "t",
			},
		},
		{
			name: "json",
			cli:  "--format json --title t",
			wants: createItemOpts{
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

			var gotOpts createItemOpts
			cmd := NewCmdCreateItem(f, func(config createItemConfig) error {
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
			assert.Equal(t, tt.wants.title, gotOpts.title)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunCreateItem_NotSupported(t *testing.T) {
	client := queries.NewTestClient()
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := createItemConfig{
		opts: createItemOpts{
			title:  "a title",
			owner:  "monalisa",
			number: 1,
		},
		client: client,
		io:     ios,
	}

	err := runCreateItem(config)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
