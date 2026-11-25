package fieldcreate

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

func TestNewCmdCreateField(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         createFieldOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "missing-name-and-data-type",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"data-type\", \"name\" not set",
		},
		{
			name:        "not-a-number",
			cli:         "x  --name n --data-type TEXT",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name:        "single-select-no-options",
			cli:         "123 --name n --data-type SINGLE_SELECT",
			wantsErr:    true,
			wantsErrMsg: "passing `--single-select-options` is required for SINGLE_SELECT data type",
		},
		{
			name: "number",
			cli:  "123 --name n --data-type TEXT",
			wants: createFieldOpts{
				number:              123,
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa --name n --data-type TEXT",
			wants: createFieldOpts{
				owner:               "monalisa",
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
			},
		},
		{
			name: "single-select-options",
			cli:  "--name n --data-type TEXT --single-select-options a,b",
			wants: createFieldOpts{
				singleSelectOptions: []string{"a", "b"},
				name:                "n",
				dataType:            "TEXT",
			},
		},
		{
			name: "json",
			cli:  "--format json --name n --data-type TEXT ",
			wants: createFieldOpts{
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
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

			var gotOpts createFieldOpts
			cmd := NewCmdCreateField(f, func(config createFieldConfig) error {
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
			assert.Equal(t, tt.wants.name, gotOpts.name)
			assert.Equal(t, tt.wants.dataType, gotOpts.dataType)
			assert.Equal(t, tt.wants.singleSelectOptions, gotOpts.singleSelectOptions)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunCreateField_NotSupported(t *testing.T) {
	client := queries.NewTestClient()
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := createFieldConfig{
		opts: createFieldOpts{
			name:     "a name",
			owner:    "monalisa",
			number:   1,
			dataType: "TEXT",
		},
		client: client,
		io:     ios,
	}

	err := runCreateField(config)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
