package itemarchive

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

func TestNewCmdarchiveItem(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         archiveItemOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "missing-id",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"id\" not set",
		},
		{
			name:        "not-a-number",
			cli:         "x --id 123",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name: "id",
			cli:  "--id 123",
			wants: archiveItemOpts{
				itemID: "123",
			},
		},
		{
			name: "number",
			cli:  "456 --id 123",
			wants: archiveItemOpts{
				number: 456,
				itemID: "123",
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa --id 123",
			wants: archiveItemOpts{
				owner:  "monalisa",
				itemID: "123",
			},
		},
		{
			name: "undo",
			cli:  "--undo  --id 123",
			wants: archiveItemOpts{
				undo:   true,
				itemID: "123",
			},
		},
		{
			name: "json",
			cli:  "--format json --id 123",
			wants: archiveItemOpts{
				itemID: "123",
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

			var gotOpts archiveItemOpts
			cmd := NewCmdArchiveItem(f, func(config archiveItemConfig) error {
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
			assert.Equal(t, tt.wants.itemID, gotOpts.itemID)
			assert.Equal(t, tt.wants.undo, gotOpts.undo)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunArchive_NotSupported(t *testing.T) {
	client := queries.NewTestClient()
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := archiveItemConfig{
		opts: archiveItemOpts{
			owner:  "monalisa",
			number: 1,
			itemID: "item ID",
		},
		client: client,
		io:     ios,
	}

	err := runArchiveItem(config)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
