package itemedit

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

func TestNewCmdeditItem(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         editItemOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "no item selector flags",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "specify the item to edit with `--id` or `--url`",
		},
		{
			name:        "invalid-flags",
			cli:         "--id 123 --text t --date 2023-01-01",
			wantsErr:    true,
			wantsErrMsg: "only one of `--text`, `--number`, `--date`, `--single-select-option-id`, `--iteration-id` or `--value` may be used",
		},
		{
			name:        "field and field-id conflict",
			cli:         "--url https://github.com/o/r/issues/1 --field Status --field-id FIELD_ID",
			wantsErr:    true,
			wantsErrMsg: "only one of `--field` or `--field-id` may be used",
		},
		{
			name:        "url and id conflict",
			cli:         "--id 123 --url https://github.com/o/r/issues/1",
			wantsErr:    true,
			wantsErrMsg: "only one of `--url` or `--id` may be used",
		},
		{
			name:        "value and single-select-option-id conflict",
			cli:         "--url https://github.com/o/r/issues/1 --field Status --value Todo --single-select-option-id OPTION_ID",
			wantsErr:    true,
			wantsErrMsg: "only one of `--text`, `--number`, `--date`, `--single-select-option-id`, `--iteration-id` or `--value` may be used",
		},
		{
			name:        "value requires field",
			cli:         "1 --url https://github.com/o/r/issues/1 --value Todo",
			wantsErr:    true,
			wantsErrMsg: "`--value` requires `--field`",
		},
		{
			name:        "value and field-id conflict",
			cli:         "1 --owner monalisa --url https://github.com/o/r/issues/1 --field-id FIELD_ID --value Todo",
			wantsErr:    true,
			wantsErrMsg: "`--value` cannot be used with `--field-id`; name the field with `--field` to use `--value`",
		},
		{
			name:        "field and id conflict",
			cli:         "1 --owner monalisa --id ITEM_ID --field Status",
			wantsErr:    true,
			wantsErrMsg: "`--field` cannot be used with `--id`; use `--url` to address the item when editing by name",
		},
		{
			name:        "name-based flags require project number",
			cli:         "--owner monalisa --url https://github.com/o/r/issues/1 --field Status --value Todo",
			wantsErr:    true,
			wantsErrMsg: "provide the project number as an argument when using `--url`, `--field`, or `--value`",
		},
		{
			name: "name-based flags",
			cli:  "1 --owner monalisa --url https://github.com/o/r/issues/1 --field Status --value Todo",
			wants: editItemOpts{
				number32:     1,
				owner:        "monalisa",
				url:          "https://github.com/o/r/issues/1",
				field:        "Status",
				value:        "Todo",
				valueChanged: true,
			},
		},
		{
			name: "item-id",
			cli:  "--id 123",
			wants: editItemOpts{
				itemID: "123",
			},
		},
		{
			name: "number",
			cli:  "--number 456 --id 123",
			wants: editItemOpts{
				number: 456,
				itemID: "123",
			},
		},
		{
			name: "number with floating point value",
			cli:  "--number 123.45 --id 123",
			wants: editItemOpts{
				number: 123.45,
				itemID: "123",
			},
		},
		{
			name: "number zero",
			cli:  "--number 0 --id 123",
			wants: editItemOpts{
				number: 0,
				itemID: "123",
			},
		},
		{
			name: "field-id",
			cli:  "--field-id FIELD_ID --id 123",
			wants: editItemOpts{
				fieldID: "FIELD_ID",
				itemID:  "123",
			},
		},
		{
			name: "project-id",
			cli:  "--project-id PROJECT_ID --id 123",
			wants: editItemOpts{
				projectID: "PROJECT_ID",
				itemID:    "123",
			},
		},
		{
			name: "text",
			cli:  "--text t --id 123",
			wants: editItemOpts{
				text:   "t",
				itemID: "123",
			},
		},
		{
			name: "date",
			cli:  "--date 2023-01-01 --id 123",
			wants: editItemOpts{
				date:   "2023-01-01",
				itemID: "123",
			},
		},
		{
			name: "single-select-option-id",
			cli:  "--single-select-option-id OPTION_ID --id 123",
			wants: editItemOpts{
				singleSelectOptionID: "OPTION_ID",
				itemID:               "123",
			},
		},
		{
			name: "iteration-id",
			cli:  "--iteration-id ITERATION_ID --id 123",
			wants: editItemOpts{
				iterationID: "ITERATION_ID",
				itemID:      "123",
			},
		},
		{
			name: "clear",
			cli:  "--id 123 --field-id FIELD_ID --project-id PROJECT_ID --clear",
			wants: editItemOpts{
				itemID:    "123",
				fieldID:   "FIELD_ID",
				projectID: "PROJECT_ID",
				clear:     true,
			},
		},
		{
			name: "json",
			cli:  "--format json --id 123",
			wants: editItemOpts{
				itemID: "123",
			},
			wantsExporter: true,
		},
		{
			name: "draft issue body only",
			cli:  "--id 123 --body foobar",
			wants: editItemOpts{
				itemID:      "123",
				body:        "foobar",
				bodyChanged: true,
			},
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

			var gotOpts editItemOpts
			cmd := NewCmdEditItem(f, func(config editItemConfig) error {
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
			assert.Equal(t, tt.wants.itemID, gotOpts.itemID)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
			assert.Equal(t, tt.wants.title, gotOpts.title)
			assert.Equal(t, tt.wants.fieldID, gotOpts.fieldID)
			assert.Equal(t, tt.wants.projectID, gotOpts.projectID)
			assert.Equal(t, tt.wants.text, gotOpts.text)
			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.date, gotOpts.date)
			assert.Equal(t, tt.wants.singleSelectOptionID, gotOpts.singleSelectOptionID)
			assert.Equal(t, tt.wants.iterationID, gotOpts.iterationID)
			assert.Equal(t, tt.wants.clear, gotOpts.clear)
			assert.Equal(t, tt.wants.titleChanged, gotOpts.titleChanged)
			assert.Equal(t, tt.wants.bodyChanged, gotOpts.bodyChanged)
			assert.Equal(t, tt.wants.body, gotOpts.body)
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wants.number32, gotOpts.number32)
			assert.Equal(t, tt.wants.url, gotOpts.url)
			assert.Equal(t, tt.wants.field, gotOpts.field)
			assert.Equal(t, tt.wants.value, gotOpts.value)
			assert.Equal(t, tt.wants.valueChanged, gotOpts.valueChanged)
		})
	}
}

func TestRunItemEdit_NoChanges(t *testing.T) {
	client := queries.NewTestClient()

	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := editItemConfig{
		io:     ios,
		opts:   editItemOpts{},
		client: client,
	}

	err := runEditItem(config)
	assert.Error(t, err, "SilentError")
	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "error: no changes to make\n", stderr.String())
}

func TestRunItemEdit_InvalidID(t *testing.T) {
	client := queries.NewTestClient()
	config := editItemConfig{
		opts: editItemOpts{
			title:        "a title",
			titleChanged: true,
			body:         "a new body",
			bodyChanged:  true,
			itemID:       "item_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.Error(t, err, "ID must be the ID of the draft issue content which is prefixed with `DI_`")
}

func TestRunItemEdit_NotSupported(t *testing.T) {
	client := queries.NewTestClient()
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			title:        "a title",
			titleChanged: true,
			itemID:       "DI_item_id",
		},
		client: client,
	}

	err := runEditItem(config)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
