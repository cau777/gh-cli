package itemadd

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

func TestNewCmdaddItem(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         addItemOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "missing-url",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"url\" not set",
		},
		{
			name:        "not-a-number",
			cli:         "x --url github.com/cli/cli",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name: "url",
			cli:  "--url github.com/cli/cli",
			wants: addItemOpts{
				itemURL: "github.com/cli/cli",
			},
		},
		{
			name: "number",
			cli:  "123 --url github.com/cli/cli",
			wants: addItemOpts{
				number:  123,
				itemURL: "github.com/cli/cli",
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa --url github.com/cli/cli",
			wants: addItemOpts{
				owner:   "monalisa",
				itemURL: "github.com/cli/cli",
			},
		},
		{
			name: "json",
			cli:  "--format json --url github.com/cli/cli",
			wants: addItemOpts{
				itemURL: "github.com/cli/cli",
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

			var gotOpts addItemOpts
			cmd := NewCmdAddItem(f, func(config addItemConfig) error {
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
			assert.Equal(t, tt.wants.itemURL, gotOpts.itemURL)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunAddItem_NotSupported(t *testing.T) {
	client := queries.NewTestClient()
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := addItemConfig{
		opts: addItemOpts{
			owner:   "monalisa",
			number:  1,
			itemURL: "https://github.com/cli/go-gh/issues/1",
		},
		client: client,
		io:     ios,
	}

	err := runAddItem(config)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
