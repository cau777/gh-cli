package itemlist

import (
	"testing"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
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
		{
			name: "query",
			cli:  `--query "assignee:octocat"`,
			wants: listOpts{
				limit: 30,
				query: "assignee:octocat",
			},
		},
		{
			name: "field",
			cli:  "--field Status --field Priority",
			wants: listOpts{
				limit:  30,
				fields: []string{"Status", "Priority"},
			},
		},
		{
			name: "field-id",
			cli:  "--field-id FIELD_ID",
			wants: listOpts{
				limit:    30,
				fieldIDs: []string{"FIELD_ID"},
			},
		},
		{
			name:        "field and field-id conflict",
			cli:         "--field Status --field-id FIELD_ID",
			wantsErr:    true,
			wantsErrMsg: "only one of `--field` or `--field-id` may be used",
		},
		{
			name:        "format and field conflict",
			cli:         "--format json --field Status",
			wantsErr:    true,
			wantsErrMsg: "cannot use `--format` with `--field` or `--field-id`",
		},
		{
			name:        "format and field-id conflict",
			cli:         "--format json --field-id FIELD_ID",
			wantsErr:    true,
			wantsErrMsg: "cannot use `--format` with `--field` or `--field-id`",
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
				assert.Error(t, err)
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wants.query, gotOpts.query)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
			assert.Equal(t, tt.wants.limit, gotOpts.limit)
			assert.Equal(t, tt.wants.fields, gotOpts.fields)
			assert.Equal(t, tt.wants.fieldIDs, gotOpts.fieldIDs)
		})
	}
}

func TestRunList_QueryUnsupported(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	config := listConfig{
		opts: listOpts{
			number: 1,
			owner:  "monalisa",
			query:  "assignee:octocat",
		},
		detector: &fd.DisabledDetectorMock{},
		io:       ios,
	}

	err := runList(config)
	assert.EqualError(t, err, "the `--query` flag is not supported on this GitHub host")
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
