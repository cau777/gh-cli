package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/attachments"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/telemetry"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdEdit(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := os.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tmpImage := filepath.Join(t.TempDir(), "shot.png")
	require.NoError(t, os.WriteFile(tmpImage, []byte("the bytes"), 0600))

	attachmentEvent := []ghtelemetry.Event{{
		Type:       "attachment_invocation",
		Dimensions: ghtelemetry.Dimensions{"command": "edit"},
		Measures: ghtelemetry.Measures{
			"attach_count": 1, "append_ops_count": 0, "replace_ops_count": 0,
		},
	}}

	tests := []struct {
		name             string
		input            string
		stdin            string
		output           EditOptions
		expectedBaseRepo ghrepo.Interface
		wantAssetPaths   []string
		wantsErr         bool
		wantsErrMsg      string
		// wantErrIsNotExist covers an error whose text the operating system
		// words differently, so the assertion cannot be on the message.
		wantErrIsNotExist bool
		wantEvents        []ghtelemetry.Event
		wantSampleRate    int
	}{
		{
			name:     "no argument",
			input:    "",
			output:   EditOptions{},
			wantsErr: true,
		},
		{
			name:  "issue number argument",
			input: "23",
			output: EditOptions{
				IssueNumbers: []int{23},
				Interactive:  true,
			},
			wantsErr: false,
		},
		{
			name:  "title flag",
			input: "23 --title test",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Title: prShared.EditableString{
						Value:  "test",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "body flag",
			input: "23 --body test",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Body: prShared.EditableString{
						Value:  "test",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "body from stdin",
			input: "23 --body-file -",
			stdin: "this is on standard input",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Body: prShared.EditableString{
						Value:  "this is on standard input",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "body from file",
			input: fmt.Sprintf("23 --body-file '%s'", tmpFile),
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Body: prShared.EditableString{
						Value:  "a body from file",
						Edited: true,
					},
				},
				BodyDir: filepath.Dir(tmpFile),
			},
			wantsErr: false,
		},
		{
			name:     "both body and body-file flags",
			input:    "23 --body foo --body-file bar",
			wantsErr: true,
		},
		{
			name:  "add-assignee flag",
			input: "23 --add-assignee monalisa,hubot",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Assignees: prShared.EditableAssignees{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-assignee flag",
			input: "23 --remove-assignee monalisa,hubot",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Assignees: prShared.EditableAssignees{
						EditableSlice: prShared.EditableSlice{
							Remove: []string{"monalisa", "hubot"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-label flag",
			input: "23 --add-label feature,TODO,bug",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Labels: prShared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-label flag",
			input: "23 --remove-label feature,TODO,bug",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Labels: prShared.EditableSlice{
						Remove: []string{"feature", "TODO", "bug"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-project flag",
			input: "23 --add-project Cleanup,Roadmap",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Projects: prShared.EditableProjects{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"Cleanup", "Roadmap"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-project flag",
			input: "23 --remove-project Cleanup,Roadmap",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Projects: prShared.EditableProjects{
						EditableSlice: prShared.EditableSlice{
							Remove: []string{"Cleanup", "Roadmap"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "milestone flag",
			input: "23 --milestone GA",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Milestone: prShared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-milestone flag",
			input: "23 --remove-milestone",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Milestone: prShared.EditableString{
						Value:  "",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:     "both milestone and remove-milestone flags",
			input:    "23 --milestone foo --remove-milestone",
			wantsErr: true,
		},
		{
			name:  "add label to multiple issues",
			input: "23 34 --add-label bug",
			output: EditOptions{
				IssueNumbers: []int{23, 34},
				Editable: prShared.Editable{
					Labels: prShared.EditableSlice{
						Add:    []string{"bug"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name: "argument is hash prefixed number",
			// Escaping is required here to avoid what I think is shellex treating it as a comment.
			input: "\\#23",
			output: EditOptions{
				IssueNumbers: []int{23},
				Interactive:  true,
			},
			wantsErr: false,
		},
		{
			name:  "argument is a URL",
			input: "https://example.com/cli/cli/issues/23",
			output: EditOptions{
				IssueNumbers: []int{23},
				Interactive:  true,
			},
			expectedBaseRepo: ghrepo.NewWithHost("cli", "cli", "example.com"),
			wantsErr:         false,
		},
		{
			name:     "URL arguments parse as different repos",
			input:    "https://github.com/cli/cli/issues/23 https://github.com/cli/go-gh/issues/23",
			wantsErr: true,
		},
		{
			name:     "interactive multiple issues",
			input:    "23 34",
			wantsErr: true,
		},
		{
			name:  "type flag",
			input: "23 --type Bug",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					IssueType: prShared.EditableString{
						Value:  "Bug",
						Edited: true,
					},
				},
			},
		},
		{
			name:  "remove-type flag",
			input: "23 --remove-type",
			output: EditOptions{
				IssueNumbers:    []int{23},
				RemoveIssueType: true,
			},
		},
		{
			name:     "both type and remove-type flags",
			input:    "23 --type Bug --remove-type",
			wantsErr: true,
		},
		{
			name:  "parent flag",
			input: "23 --parent 100",
			output: EditOptions{
				IssueNumbers: []int{23},
				Parent:       "100",
			},
		},
		{
			name:  "remove-parent flag",
			input: "23 --remove-parent",
			output: EditOptions{
				IssueNumbers: []int{23},
				RemoveParent: true,
			},
		},
		{
			name:     "both parent and remove-parent flags",
			input:    "23 --parent 100 --remove-parent",
			wantsErr: true,
		},
		{
			name:  "add-sub-issue flag",
			input: "23 --add-sub-issue 123,124",
			output: EditOptions{
				IssueNumbers: []int{23},
				AddSubIssues: []string{"123", "124"},
			},
		},
		{
			name:     "add-sub-issue rejected with multiple issues",
			input:    "23 24 --add-sub-issue 123",
			wantsErr: true,
		},
		{
			name:  "remove-sub-issue flag",
			input: "23 --remove-sub-issue 50",
			output: EditOptions{
				IssueNumbers:    []int{23},
				RemoveSubIssues: []string{"50"},
			},
		},
		{
			name:  "add-blocked-by flag",
			input: "23 --add-blocked-by 200",
			output: EditOptions{
				IssueNumbers: []int{23},
				AddBlockedBy: []string{"200"},
			},
		},
		{
			name:  "remove-blocked-by flag",
			input: "23 --remove-blocked-by 201",
			output: EditOptions{
				IssueNumbers:    []int{23},
				RemoveBlockedBy: []string{"201"},
			},
		},
		{
			name:  "add-blocking flag",
			input: "23 --add-blocking 300,301",
			output: EditOptions{
				IssueNumbers: []int{23},
				AddBlocking:  []string{"300", "301"},
			},
		},
		{
			name:  "remove-blocking flag",
			input: "23 --remove-blocking 300",
			output: EditOptions{
				IssueNumbers:   []int{23},
				RemoveBlocking: []string{"300"},
			},
		},
		{
			name:  "attach flag",
			input: fmt.Sprintf("23 --attach '%s'", tmpImage),
			output: EditOptions{
				IssueNumbers: []int{23},
				Interactive:  false,
			},
			wantAssetPaths: []string{tmpImage},
			wantEvents:     attachmentEvent,
			wantSampleRate: ghtelemetry.SAMPLE_ALL,
		},
		{
			name:        "argument validation skips attachment telemetry",
			input:       fmt.Sprintf("--attach '%s'", tmpImage),
			wantsErr:    true,
			wantsErrMsg: "requires at least 1 arg(s), only received 0",
		},
		{
			name:  "attach flag beside another edit flag",
			input: fmt.Sprintf("23 --add-label bug --attach '%s'", tmpImage),
			output: EditOptions{
				IssueNumbers: []int{23},
				Interactive:  false,
				Editable: prShared.Editable{
					Labels: prShared.EditableSlice{
						Add:    []string{"bug"},
						Edited: true,
					},
				},
			},
			wantAssetPaths: []string{tmpImage},
			wantEvents:     attachmentEvent,
			wantSampleRate: ghtelemetry.SAMPLE_ALL,
		},
		{
			name:           "attach flag with more than one issue",
			input:          fmt.Sprintf("23 34 --attach '%s'", tmpImage),
			wantsErr:       true,
			wantsErrMsg:    "`--attach` cannot be used when editing multiple issues",
			wantEvents:     attachmentEvent,
			wantSampleRate: ghtelemetry.SAMPLE_ALL,
		},
		{
			name:        "body flag conflict skips attachment telemetry",
			input:       fmt.Sprintf("23 --body test --body-file '%s' --attach '%s'", tmpFile, tmpImage),
			wantsErr:    true,
			wantsErrMsg: "specify only one of `--body` or `--body-file`",
		},
		{
			name:              "attach flag naming a file that does not exist",
			input:             "23 --attach ./nope.png",
			wantsErr:          true,
			wantsErrMsg:       "./nope.png: ",
			wantErrIsNotExist: true,
			wantEvents:        attachmentEvent,
			wantSampleRate:    ghtelemetry.SAMPLE_ALL,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given command inputs and an invocation recorder
			ios, stdin, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.input)
			require.NoError(t, err)

			var gotOpts *EditOptions
			recorder := &telemetry.InvocationRecorderSpy{}
			cmd := NewCmdEdit(f, recorder, func(opts *EditOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			// When the command executes
			_, err = cmd.ExecuteC()
			// Then telemetry starts only if execution reaches attachment validation
			assert.Equal(t, tt.wantEvents, recorder.Events())
			assert.Equal(t, tt.wantSampleRate, recorder.LastSampleRate)
			if tt.wantsErr {
				require.Error(t, err)
				if tt.wantsErrMsg != "" {
					if tt.wantErrIsNotExist {
						require.ErrorIs(t, err, fs.ErrNotExist)
						require.ErrorContains(t, err, tt.wantsErrMsg)
					} else {
						require.EqualError(t, err, tt.wantsErrMsg)
					}
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.output.IssueNumbers, gotOpts.IssueNumbers)
			assert.Equal(t, tt.output.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.output.Editable, gotOpts.Editable)
			assert.Equal(t, tt.output.BodyDir, gotOpts.BodyDir)
			assert.Equal(t, tt.output.Parent, gotOpts.Parent)
			assert.Equal(t, tt.output.RemoveParent, gotOpts.RemoveParent)
			assert.Equal(t, tt.output.AddSubIssues, gotOpts.AddSubIssues)
			assert.Equal(t, tt.output.RemoveSubIssues, gotOpts.RemoveSubIssues)
			assert.Equal(t, tt.output.AddBlockedBy, gotOpts.AddBlockedBy)
			assert.Equal(t, tt.output.RemoveBlockedBy, gotOpts.RemoveBlockedBy)
			assert.Equal(t, tt.output.AddBlocking, gotOpts.AddBlocking)
			assert.Equal(t, tt.output.RemoveBlocking, gotOpts.RemoveBlocking)

			var assetPaths []string
			for _, a := range gotOpts.Assets {
				assetPaths = append(assetPaths, a.Path())
			}
			assert.Equal(t, tt.wantAssetPaths, assetPaths)

			if tt.expectedBaseRepo != nil {
				baseRepo, err := gotOpts.BaseRepo()
				require.NoError(t, err)
				require.True(
					t,
					ghrepo.IsSame(tt.expectedBaseRepo, baseRepo),
					"expected base repo %+v, got %+v", tt.expectedBaseRepo, baseRepo,
				)
			}
		})
	}
}

func Test_editRun(t *testing.T) {
	tests := []struct {
		name      string
		input     *EditOptions
		httpStubs func(*testing.T, *httpmock.Registry)
		attach    []string
		host      string
		uploads   []attachments.UploadStub
		// The lookup builds its field list from a set, so the order varies and
		// a row cannot assert the whole query.
		wantLookupSelections   []string
		wantNoLookupSelections []string
		stdout                 string
		stderr                 string
		wantErr                bool
		wantErrMsg             string
		// Used instead of wantErrMsg when the subject is which errors survive,
		// leaving their wording to the layer that formats them.
		wantErrContains []string
		wantOperations  *attachments.UploadResult
	}{
		{
			name: "non-interactive",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				Title: prShared.EditableString{
					Value:  "new title",
					Edited: true,
				},
				Body: prShared.EditableString{
					Value:  "new body",
					Edited: true,
				},
				Assignees: prShared.EditableAssignees{
					EditableSlice: prShared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
					},
				},
				Labels: prShared.EditableSlice{
					Add:    []string{"feature", "TODO", "bug"},
					Remove: []string{"docs"},
					Edited: true,
				},
				Milestone: prShared.EditableString{
					Value:  "GA",
					Edited: true,
				},
				Metadata: api.RepoMetadataResult{
					Labels: []api.RepoLabel{
						{Name: "docs", ID: "DOCSID"},
					},
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				mockRepoMetadata(t, reg)
				mockIssueUpdate(t, reg)
				mockIssueUpdateLabels(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "non-interactive projects unsupported",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				Projects: prShared.EditableProjects{
					EditableSlice: prShared.EditableSlice{
						Add:    []string{"Cleanup"},
						Edited: true,
					},
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs:  func(t *testing.T, reg *httpmock.Registry) {},
			wantErr:    true,
			wantErrMsg: api.NotSupportedError("editing projects on an issue is not supported with REST").Error(),
		},
		{
			name: "non-interactive multiple issues",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{456, 123},
				Interactive:  false,
				Assignees: prShared.EditableAssignees{
					EditableSlice: prShared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
					},
				},
				Labels: prShared.EditableSlice{
					Add:    []string{"feature", "TODO", "bug"},
					Remove: []string{"docs"},
					Edited: true,
				},
				Milestone: prShared.EditableString{
					Value:  "GA",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Should only be one fetch of metadata.
				mockRepoMetadata(t, reg)
				// All other queries and mutations should be doubled.
				mockIssueNumberGet(t, reg, 123)
				mockIssueNumberGet(t, reg, 456)
				mockIssueUpdateForNumber(t, reg, 123)
				mockIssueUpdateForNumber(t, reg, 456)
				mockIssueUpdateLabelsForNumber(t, reg, 123)
				mockIssueUpdateLabelsForNumber(t, reg, 456)
			},
			stdout: heredoc.Doc(`
				https://github.com/OWNER/REPO/issue/123
				https://github.com/OWNER/REPO/issue/456
			`),
		},
		{
			name: "non-interactive multiple issues with fetch failures",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123, 9999},
				Interactive:  false,
				Assignees: prShared.EditableAssignees{
					EditableSlice: prShared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
					},
				},
				Labels: prShared.EditableSlice{
					Add:    []string{"feature", "TODO", "bug"},
					Remove: []string{"docs"},
					Edited: true,
				},
				Milestone: prShared.EditableString{
					Value:  "GA",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueNumberGet(t, reg, 123)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/9999"),
					httpmock.StatusJSONResponse(404, map[string]string{"message": "Not Found"}),
				)
			},
			wantErr: true,
		},
		{
			name: "non-interactive multiple issues with update failures",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123, 456},
				Interactive:  false,
				Assignees: prShared.EditableAssignees{
					EditableSlice: prShared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
					},
				},
				Milestone: prShared.EditableString{
					Value:  "GA",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Should only be one fetch of metadata.
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/milestones"),
					httpmock.StringResponse(`[
						{ "title": "GA", "node_id": "GAID", "number": 1 },
						{ "title": "Big One.oh", "node_id": "BIGONEID", "number": 2 }
					]`))
				// All other queries should be doubled.
				mockIssueNumberGet(t, reg, 123)
				mockIssueNumberGet(t, reg, 456)
				// Updating 123 should succeed.
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.StatusStringResponse(200, `{}`),
				)
				// Updating 456 should fail.
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/456"),
					httpmock.StatusJSONResponse(500, map[string]string{"message": "test error"}),
				)
			},
			stdout: heredoc.Doc(`
				https://github.com/OWNER/REPO/issue/123
			`),
			stderr:  `failed to update https://github.com/OWNER/REPO/issue/456:.*test error`,
			wantErr: true,
		},
		{
			name: "interactive",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  true,
				FieldsToEditSurvey: func(p prShared.EditPrompter, eo *prShared.Editable) error {
					eo.Title.Edited = true
					eo.Body.Edited = true
					eo.Assignees.Edited = true
					eo.Labels.Edited = true
					eo.Milestone.Edited = true
					return nil
				},
				EditFieldsSurvey: func(p prShared.EditPrompter, eo *prShared.Editable, _ string) error {
					eo.Title.Value = "new title"
					eo.Body.Value = "new body"
					eo.Assignees.Value = []string{"monalisa", "hubot"}
					eo.Labels.Value = []string{"feature", "TODO", "bug"}
					eo.Labels.Add = []string{"feature", "TODO", "bug"}
					eo.Labels.Remove = []string{"docs"}
					eo.Milestone.Value = "GA"
					return nil
				},
				FetchOptions:    prShared.FetchOptions,
				DetermineEditor: func() (string, error) { return "vim", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/assignees"),
					httpmock.StringResponse(`[
						{ "login": "hubot" },
						{ "login": "monalisa" }
					]`),
				)
				mockRepoMetadata(t, reg)
				mockIssueUpdate(t, reg)
				mockIssueUpdateLabels(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "interactive prompts with assignee logins when actors available",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  true,
				FieldsToEditSurvey: func(p prShared.EditPrompter, eo *prShared.Editable) error {
					eo.Assignees.Edited = true
					return nil
				},
				EditFieldsSurvey: func(p prShared.EditPrompter, eo *prShared.Editable, _ string) error {
					require.Equal(t, []string{"hubot"}, eo.Assignees.Default)

					eo.Assignees.Value = []string{"hubot", "MonaLisa"}
					return nil
				},
				FetchOptions:    prShared.FetchOptions,
				DetermineEditor: func() (string, error) { return "vim", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIsssueNumberGetWithAssignedActors(t, reg, 123)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/assignees"),
					httpmock.StringResponse(`[
						{ "login": "hubot" },
						{ "login": "MonaLisa" }
					]`),
				)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							require.Subset(t, inputs["assignees"], []any{"hubot", "MonaLisa"})
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "interactive prompts with user assignee logins when actors unavailable",
			input: &EditOptions{
				IssueNumbers: []int{123},
				Interactive:  true,
				FieldsToEditSurvey: func(p prShared.EditPrompter, eo *prShared.Editable) error {
					eo.Assignees.Edited = true
					return nil
				},
				EditFieldsSurvey: func(p prShared.EditPrompter, eo *prShared.Editable, _ string) error {
					// Checking that only the login is used in the prompt (no display name)
					require.Equal(t, eo.Assignees.Default, []string{"hubot", "MonaLisa"})

					// Mocking a selection of only MonaLisa in the prompt.
					eo.Assignees.Value = []string{"MonaLisa"}
					return nil
				},
				FetchOptions:    prShared.FetchOptions,
				DetermineEditor: func() (string, error) { return "vim", nil },
				Detector:        &fd.DisabledDetectorMock{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/123"),
					httpmock.StringResponse(`{
						"id": 123,
						"node_id": "123",
						"number": 123,
						"html_url": "https://github.com/OWNER/REPO/issue/123",
						"assignees": [
							{ "node_id": "HUBOTID", "login": "hubot", "type": "User" },
							{ "node_id": "MONAID", "login": "MonaLisa", "name": "Mona Display Name", "type": "User" }
						]
					}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/assignees"),
					httpmock.StringResponse(`[
						{ "login": "hubot", "node_id": "HUBOTID" },
						{ "login": "MonaLisa", "node_id": "MONAID" }
					]`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							require.Contains(t, inputs["assignees"], "MonaLisa")
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit type",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				IssueType: prShared.EditableString{
					Value:  "Bug",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.REST("GET", "orgs/OWNER/issue-types"),
					httpmock.StringResponse(`[
						{ "node_id": "BUG_TYPE_ID", "name": "Bug", "description": "", "color": "" },
						{ "node_id": "FEATURE_TYPE_ID", "name": "Feature", "description": "", "color": "" }
					]`),
				)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							assert.Equal(t, "Bug", inputs["type"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "an invalid issue type fails before upload",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				IssueType: prShared.EditableString{
					Value:  "NotAType",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			attach: []string{"shot.png"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				reg.Register(
					httpmock.REST("GET", "orgs/OWNER/issue-types"),
					httpmock.StringResponse(`[
						{ "node_id": "BUG_TYPE_ID", "name": "Bug", "description": "", "color": "" }
					]`),
				)
				reg.Exclude(t, httpmock.REST("POST", "user-attachments/assets"))
				reg.Exclude(t, httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"))
			},
			wantErr:    true,
			wantErrMsg: `type "NotAType" not found; available types: Bug`,
		},
		{
			name: "remove type",
			input: &EditOptions{
				Detector:        &fd.EnabledDetectorMock{},
				IssueNumbers:    []int{123},
				Interactive:     false,
				RemoveIssueType: true,
				FetchOptions:    prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							v, exists := inputs["type"]
							assert.True(t, exists)
							assert.Nil(t, v)
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "interactive edit type prompt",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  true,
				FieldsToEditSurvey: func(_ prShared.EditPrompter, eo *prShared.Editable) error {
					// Verify the survey is allowed to offer Type as an option for issue edit.
					assert.True(t, eo.IssueType.Selectable)
					eo.IssueType.Edited = true
					return nil
				},
				EditFieldsSurvey: func(_ prShared.EditPrompter, eo *prShared.Editable, _ string) error {
					// FetchOptions populated Options and IssueTypeNameToID from
					// the issue types stub below.
					assert.Equal(t, []string{"Bug", "Feature"}, eo.IssueType.Options)
					assert.Equal(t, "FEATURE_TYPE_ID", eo.IssueTypeNameToID["Feature"])
					eo.IssueType.Value = "Feature"
					return nil
				},
				FetchOptions:    prShared.FetchOptions,
				DetermineEditor: func() (string, error) { return "vim", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.REST("GET", "orgs/OWNER/issue-types"),
					httpmock.StringResponse(`[
						{ "node_id": "BUG_TYPE_ID", "name": "Bug", "description": "", "color": "" },
						{ "node_id": "FEATURE_TYPE_ID", "name": "Feature", "description": "", "color": "" }
					]`),
				)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							assert.Equal(t, "Feature", inputs["type"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit set parent",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				Parent:       "100",
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/123/parent"),
					httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/100"),
					httpmock.StringResponse(`{ "id": 100, "node_id": "PARENT_100_ID", "number": 100 }`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/issues/100/sub_issues"),
					httpmock.RESTPayload(201, `{}`,
						func(inputs map[string]any) {
							assert.Equal(t, float64(123), inputs["sub_issue_id"])
							assert.Equal(t, true, inputs["replace_parent"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit remove parent",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				RemoveParent: true,
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/123"),
					httpmock.StringResponse(`{ "id": 123, "node_id": "123", "number": 123, "html_url": "https://github.com/OWNER/REPO/issue/123" }`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/123/parent"),
					httpmock.StringResponse(`{
						"id": 100,
						"node_id": "PARENT_100_ID",
						"number": 100,
						"title": "Parent Issue",
						"html_url": "https://github.com/OWNER/REPO/issues/100",
						"state": "open"
					}`),
				)
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/issues/100/sub_issue"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							assert.Equal(t, float64(123), inputs["sub_issue_id"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit add sub-issues",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{100},
				Interactive:  false,
				AddSubIssues: []string{"123", "124"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueNumberGet(t, reg, 100)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/123"),
					httpmock.StringResponse(`{ "id": 123, "node_id": "SUB_123_ID", "number": 123 }`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/124"),
					httpmock.StringResponse(`{ "id": 124, "node_id": "SUB_124_ID", "number": 124 }`),
				)
				reg.Register(
					restJSONMatcher("POST", "repos/OWNER/REPO/issues/100/sub_issues", func(input map[string]any) bool {
						return input["sub_issue_id"] == float64(123) && input["replace_parent"] == true
					}),
					httpmock.StatusStringResponse(201, `{}`),
				)
				reg.Register(
					restJSONMatcher("POST", "repos/OWNER/REPO/issues/100/sub_issues", func(input map[string]any) bool {
						return input["sub_issue_id"] == float64(124) && input["replace_parent"] == true
					}),
					httpmock.StatusStringResponse(201, `{}`),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/100\n",
		},
		{
			name: "edit remove sub-issue",
			input: &EditOptions{
				Detector:        &fd.EnabledDetectorMock{},
				IssueNumbers:    []int{100},
				Interactive:     false,
				RemoveSubIssues: []string{"123"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueNumberGet(t, reg, 100)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/123"),
					httpmock.StringResponse(`{ "id": 123, "node_id": "SUB_123_ID", "number": 123 }`),
				)
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/issues/100/sub_issue"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							assert.Equal(t, float64(123), inputs["sub_issue_id"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/100\n",
		},
		{
			name: "edit add and remove blocked-by",
			input: &EditOptions{
				Detector:        &fd.EnabledDetectorMock{},
				IssueNumbers:    []int{123},
				Interactive:     false,
				AddBlockedBy:    []string{"200"},
				RemoveBlockedBy: []string{"201"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/200"),
					httpmock.StringResponse(`{ "id": 200, "node_id": "BLOCKING_200_ID", "number": 200 }`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/issues/123/dependencies/blocked_by"),
					httpmock.RESTPayload(201, `{}`,
						func(inputs map[string]any) {
							assert.Equal(t, float64(200), inputs["issue_id"])
						}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/201"),
					httpmock.StringResponse(`{ "id": 201, "node_id": "BLOCKING_201_ID", "number": 201 }`),
				)
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/issues/123/dependencies/blocked_by/201"),
					httpmock.StatusStringResponse(200, `{}`),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit add blocking swaps args",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				AddBlocking:  []string{"300"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/300"),
					httpmock.StringResponse(`{ "id": 300, "node_id": "BLOCKED_300_ID", "number": 300 }`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/issues/300/dependencies/blocked_by"),
					httpmock.RESTPayload(201, `{}`,
						func(inputs map[string]any) {
							assert.Equal(t, float64(123), inputs["issue_id"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit remove blocking swaps args",
			input: &EditOptions{
				Detector:       &fd.EnabledDetectorMock{},
				IssueNumbers:   []int{123},
				Interactive:    false,
				RemoveBlocking: []string{"300"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/300"),
					httpmock.StringResponse(`{ "id": 300, "node_id": "BLOCKED_300_ID", "number": 300 }`),
				)
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/issues/300/dependencies/blocked_by/123"),
					httpmock.StatusStringResponse(200, `{}`),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "batch edit type",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123, 456},
				Interactive:  false,
				IssueType: prShared.EditableString{
					Value:  "Bug",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "orgs/OWNER/issue-types"),
					httpmock.StringResponse(`[
						{ "node_id": "BUG_TYPE_ID", "name": "Bug", "description": "", "color": "" }
					]`),
				)
				mockIssueNumberGet(t, reg, 123)
				mockIssueNumberGet(t, reg, 456)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.RESTPayload(200, `{}`, func(inputs map[string]any) {
						assert.Equal(t, "Bug", inputs["type"])
					}),
				)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/456"),
					httpmock.RESTPayload(200, `{}`, func(inputs map[string]any) {
						assert.Equal(t, "Bug", inputs["type"])
					}),
				)
			},
			stdout: heredoc.Doc(`
				https://github.com/OWNER/REPO/issue/123
				https://github.com/OWNER/REPO/issue/456
			`),
		},
		{
			name: "attaching with no body flag keeps the body already on the issue",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				FetchOptions: prShared.FetchOptions,
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				mockIssueUpdateWithBody(t, reg, "the original body\n\n![shot](https://example.com/1)")
			},
			stdout:         "https://github.com/OWNER/REPO/issue/123\n",
			wantOperations: &attachments.UploadResult{AppendOperations: 1},
		},
		{
			name: "a body flag replaces the body the attachment is then appended to",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Body: prShared.EditableString{
					Value:  "a new body",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				mockIssueUpdateWithBody(t, reg, "a new body\n\n![shot](https://example.com/1)")
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "attaching rewrites a reference relative to the body file",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Body: prShared.EditableString{
					Value:  "the plan ![diagram](./diagram.png)",
					Edited: true,
				},
				BodyDir:      "docs",
				FetchOptions: prShared.FetchOptions,
			},
			attach:  []string{"docs/diagram.png"},
			uploads: []attachments.UploadStub{{Name: "diagram.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				mockIssueUpdateWithBody(t, reg, "the plan ![diagram](https://example.com/1)")
			},
			stdout:         "https://github.com/OWNER/REPO/issue/123\n",
			wantOperations: &attachments.UploadResult{ReplaceOperations: 1},
		},
		{
			name: "an empty body flag clears the body and leaves the attachment",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Body: prShared.EditableString{
					Value:  "",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				mockIssueUpdateWithBody(t, reg, "![shot](https://example.com/1)")
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "a run that attaches nothing does not write the body",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Title: prShared.EditableString{
					Value:  "a new title",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/123"),
					httpmock.StringResponse(`{"id":123,"node_id":"123","number":123,"html_url":"https://github.com/OWNER/REPO/issue/123","body":"the original body"}`),
				)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							assert.NotContains(t, inputs, "body")
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "a permission that cannot upload fails before the write",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				FetchOptions: prShared.FetchOptions,
			},
			attach: []string{"shot.png"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "READ")
				reg.Exclude(t, httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"))
			},
			wantErr:    true,
			wantErrMsg: "attaching files requires write access to the repository",
		},
		{
			name: "a missing repository id fails before the write",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				FetchOptions: prShared.FetchOptions,
			},
			attach: []string{"shot.png"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 0, "WRITE")
				reg.Exclude(t, httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"))
			},
			wantErr:    true,
			wantErrMsg: "could not determine which repository to attach files to",
		},
		{
			name: "attaching asks the issue lookup for the repository fields",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				FetchOptions: prShared.FetchOptions,
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				mockIssueUpdate(t, reg)
			},
			wantLookupSelections: []string{"GET /repos/OWNER/REPO\n"},
			stdout:               "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "the lookup does not ask for the repository fields without an attachment",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Title: prShared.EditableString{
					Value:  "a new title",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				mockIssueUpdate(t, reg)
			},
			wantNoLookupSelections: []string{"GET /repos/OWNER/REPO\n"},
			stdout:                 "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "a partial upload writes the body so the uploaded asset is not orphaned",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				FetchOptions: prShared.FetchOptions,
			},
			attach: []string{"first.png", "second.png"},
			uploads: []attachments.UploadStub{
				{Name: "first.png", Status: 201, Body: `{"url":"https://example.com/1"}`},
				{Name: "second.png", Status: 404, Body: `{}`},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				mockIssueUpdateWithBody(t, reg, "the original body\n\n![first](https://example.com/1)")
			},
			stdout:          "https://github.com/OWNER/REPO/issue/123\n",
			wantErr:         true,
			wantErrContains: []string{"./second.png"},
			wantOperations:  &attachments.UploadResult{AppendOperations: 1},
		},
		{
			name: "a sole failed upload does not write the body",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				FetchOptions: prShared.FetchOptions,
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 404, Body: `{}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				reg.Exclude(t, httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"))
			},
			wantErr:         true,
			wantErrContains: []string{"./shot.png"},
		},
		{
			name: "a body that fails validation is left alone",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				FetchOptions: prShared.FetchOptions,
			},
			attach: []string{"clip.mp4"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "![clip][ref]\n\n[ref]: ./clip.mp4", 1234, "WRITE")
				reg.Exclude(t, httpmock.REST("POST", "user-attachments/assets"))
				reg.Exclude(t, httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"))
			},
			wantErr:         true,
			wantErrContains: []string{"cannot embed a video as a reference-style image"},
		},
		{
			name: "a body flag is not written on its own when nothing uploaded",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Body: prShared.EditableString{
					Value:  "See below",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 404, Body: `{}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				reg.Exclude(t, httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"))
			},
			wantErr:         true,
			wantErrContains: []string{"./shot.png"},
		},
		{
			name: "another field is written when nothing uploaded",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Title: prShared.EditableString{
					Value:  "a new title",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 404, Body: `{}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							assert.Equal(t, "a new title", inputs["title"])
							assert.NotContains(t, inputs, "body")
						}),
				)
			},
			stdout:          "https://github.com/OWNER/REPO/issue/123\n",
			wantErr:         true,
			wantErrContains: []string{"./shot.png"},
		},
		{
			name: "a deferred edit is written when nothing uploaded",
			input: &EditOptions{
				Detector:        &fd.EnabledDetectorMock{},
				IssueNumbers:    []int{123},
				RemoveIssueType: true,
				FetchOptions:    prShared.FetchOptions,
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 404, Body: `{}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.RESTPayload(200, `{}`,
						func(inputs map[string]any) {
							v, exists := inputs["type"]
							assert.True(t, exists)
							assert.Nil(t, v)
						}),
				)
			},
			stdout:          "https://github.com/OWNER/REPO/issue/123\n",
			wantErr:         true,
			wantErrContains: []string{"./shot.png"},
		},
		{
			name: "an interactive run that edits nothing still reports the issue",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  true,
				FieldsToEditSurvey: func(p prShared.EditPrompter, eo *prShared.Editable) error {
					return nil
				},
				EditFieldsSurvey: func(p prShared.EditPrompter, eo *prShared.Editable, _ string) error {
					return nil
				},
				DetermineEditor: func() (string, error) { return "vim", nil },
				FetchOptions:    prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Exclude(t, httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"))
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "the upload goes to the host the issue was fetched from",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				FetchOptions: prShared.FetchOptions,
			},
			attach: []string{"shot.png"},
			// A ghe.com tenant rather than a GHES host, which NewUploader
			// refuses.
			host:    "acme.ghe.com",
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				mockIssueUpdateWithBody(t, reg, "the original body\n\n![shot](https://example.com/1)")
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "a failed write does not swallow a failed upload",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Title: prShared.EditableString{
					Value:  "a new title",
					Edited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 413, Body: `{}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGetWithRepository(reg, "the original body", 1234, "WRITE")
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
					httpmock.StatusStringResponse(500, `{}`),
				)
			},
			stderr:          `failed to update https://github\.com/OWNER/REPO/issue/123`,
			wantErr:         true,
			wantErrContains: []string{"./shot.png", "failed to update 1 issue"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			host := tt.host
			if host == "" {
				host = "github.com"
			}

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			for _, u := range tt.uploads {
				attachments.StubUploadToHost(t, reg, "uploads."+host, 1234, u.Name, u.Status, u.Body)
			}
			tt.httpStubs(t, reg)

			recorder := &graphQLRecorder{inner: reg}
			httpClient := func() (*http.Client, error) {
				return &http.Client{Transport: recorder}, nil
			}
			baseRepo := func() (ghrepo.Interface, error) {
				return ghrepo.NewWithHost("OWNER", "REPO", host), nil
			}

			tt.input.IO = ios
			tt.input.HttpClient = httpClient
			tt.input.BaseRepo = baseRepo

			// NewTestAssets moves into a temporary directory, so it runs before
			// anything else reads a relative path.
			if len(tt.attach) > 0 {
				tt.input.Assets = attachments.NewTestAssets(t, tt.attach...)
			}
			// Given a pending event when operation counts are under test
			attachmentRecorder := &telemetry.InvocationRecorderSpy{}
			if tt.wantOperations != nil {
				tt.input.AttachEvent = attachments.BeginTelemetry(attachmentRecorder, "gh test", len(tt.attach))
			}

			// When issue editing runs
			err := editRun(tt.input)
			if tt.wantOperations != nil {
				// Then telemetry retains completed operations, including partial results
				attachments.AssertTestTelemetryEvents(t, attachmentRecorder.Events(), len(tt.attach), *tt.wantOperations)
			}
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrMsg != "" {
					require.EqualError(t, err, tt.wantErrMsg)
				}
				for _, substring := range tt.wantErrContains {
					assert.ErrorContains(t, err, substring)
				}
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.stdout, stdout.String())
			// Use regex match since mock errors and service errors will differ.
			assert.Regexp(t, tt.stderr, stderr.String())

			queries := recorder.recorded()
			for _, selection := range tt.wantLookupSelections {
				assert.Contains(t, queries, selection)
			}
			for _, selection := range tt.wantNoLookupSelections {
				assert.NotContains(t, queries, selection)
			}
		})
	}
}

// graphQLRecorder records the GraphQL queries a run sends, then delegates to
// the registry it wraps. A run can look up several things at once, so the
// mutex guards the recorded queries against concurrent requests.
type graphQLRecorder struct {
	inner http.RoundTripper

	mu      sync.Mutex
	queries []string
}

func (g *graphQLRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL != nil {
		g.mu.Lock()
		g.queries = append(g.queries, req.Method+" "+req.URL.Path)
		g.mu.Unlock()
	}
	if req.Body != nil && strings.HasSuffix(req.URL.Path, "/graphql") {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		g.mu.Lock()
		g.queries = append(g.queries, string(body))
		g.mu.Unlock()
	}
	return g.inner.RoundTrip(req)
}

// recorded returns every query seen so far, joined for substring matching.
func (g *graphQLRecorder) recorded() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return "\n" + strings.Join(g.queries, "\n") + "\n"
}

func mockIssueGet(_ *testing.T, reg *httpmock.Registry) {
	mockIssueNumberGet(nil, reg, 123)
}

// mockIssueGetWithRepository stubs the issue lookup with a body and the two
// repository fields the uploader reads. A row states both, so an empty
// permission is a value a row can ask for.
func mockIssueGetWithRepository(reg *httpmock.Registry, body string, databaseID int64, viewerPermission string) {
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/issues/123"),
		httpmock.StringResponse(fmt.Sprintf(`{
			"id": 123,
			"node_id": "123",
			"number": 123,
			"html_url": "https://github.com/OWNER/REPO/issue/123",
			"body": %s
		}`, strconv.Quote(body))),
	)
	permKey := ""
	switch viewerPermission {
	case "ADMIN":
		permKey = `"admin": true`
	case "MAINTAIN":
		permKey = `"maintain": true`
	case "WRITE":
		permKey = `"push": true`
	case "TRIAGE":
		permKey = `"triage": true`
	case "READ":
		permKey = `"pull": true`
	}
	permsJSON := `{}`
	if permKey != "" {
		permsJSON = fmt.Sprintf(`{%s}`, permKey)
	}
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO"),
		httpmock.StringResponse(fmt.Sprintf(`{
			"id": %d,
			"name": "REPO",
			"full_name": "OWNER/REPO",
			"permissions": %s
		}`, databaseID, permsJSON)),
	)
}

func mockIssueNumberGet(_ *testing.T, reg *httpmock.Registry, number int) {
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/OWNER/REPO/issues/%d", number)),
		httpmock.StringResponse(fmt.Sprintf(`{
			"id": %[1]d,
			"node_id": "%[1]d",
			"number": %[1]d,
			"html_url": "https://github.com/OWNER/REPO/issue/%[1]d",
			"labels": [
				{ "node_id": "DOCSID", "name": "docs" }
			]
		}`, number)),
	)
}

func mockIsssueNumberGetWithAssignedActors(_ *testing.T, reg *httpmock.Registry, number int) {
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/OWNER/REPO/issues/%d", number)),
		httpmock.StringResponse(fmt.Sprintf(`{
			"id": %[1]d,
			"node_id": "%[1]d",
			"number": %[1]d,
			"html_url": "https://github.com/OWNER/REPO/issue/%[1]d",
			"assignees": [
				{
					"node_id": "HUBOTID",
					"login": "hubot",
					"type": "Bot"
				}
			]
		}`, number)),
	)
}

func mockRepoMetadata(_ *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/labels"),
		httpmock.StringResponse(`[
			{ "name": "feature", "node_id": "FEATUREID" },
			{ "name": "TODO", "node_id": "TODOID" },
			{ "name": "bug", "node_id": "BUGID" },
			{ "name": "docs", "node_id": "DOCSID" }
		]`))
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/milestones"),
		httpmock.StringResponse(`[
			{ "title": "GA", "node_id": "GAID", "number": 1 },
			{ "title": "Big One.oh", "node_id": "BIGONEID", "number": 2 }
		]`))
}

func mockIssueUpdate(t *testing.T, reg *httpmock.Registry) {
	mockIssueUpdateForNumber(t, reg, 123)
}

func mockIssueUpdateForNumber(_ *testing.T, reg *httpmock.Registry, number int) {
	reg.Register(
		httpmock.REST("PATCH", fmt.Sprintf("repos/OWNER/REPO/issues/%d", number)),
		httpmock.StatusStringResponse(200, `{}`),
	)
}

// mockIssueUpdateWithBody stubs the issue update and asserts the body it
// carries.
func mockIssueUpdateWithBody(t *testing.T, reg *httpmock.Registry, wantBody string) {
	reg.Register(
		httpmock.REST("PATCH", "repos/OWNER/REPO/issues/123"),
		httpmock.RESTPayload(200, `{}`,
			func(inputs map[string]any) {
				assert.Equal(t, wantBody, inputs["body"])
			}),
	)
}

func mockIssueUpdateLabels(t *testing.T, reg *httpmock.Registry) {
	mockIssueUpdateLabelsForNumber(t, reg, 123)
}

func mockIssueUpdateLabelsForNumber(_ *testing.T, reg *httpmock.Registry, number int) {
	reg.Register(
		httpmock.REST("POST", fmt.Sprintf("repos/OWNER/REPO/issues/%d/labels", number)),
		httpmock.StatusStringResponse(200, `[]`),
	)
	reg.Register(
		httpmock.REST("DELETE", fmt.Sprintf("repos/OWNER/REPO/issues/%d/labels/docs", number)),
		httpmock.StatusStringResponse(200, `[]`),
	)
}

func restJSONMatcher(method, path string, match func(map[string]any) bool) httpmock.Matcher {
	return func(req *http.Request) bool {
		if !httpmock.REST(method, path)(req) || req.Body == nil {
			return false
		}
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return false
		}
		req.Body = io.NopCloser(bytes.NewReader(b))
		var data map[string]any
		if err := json.Unmarshal(b, &data); err != nil {
			return false
		}
		return match(data)
	}
}

// Test_editRun_crossHostRelationshipRefs verifies that every relationship
// flag rejects a cross-host issue URL with the same clear error. Lives as
// its own table rather than additional cases in Test_editRun because each
// case shares identical setup and asserts the same error, varying only in
// which input field carries the cross-host URL.
func Test_editRun_crossHostRelationshipRefs(t *testing.T) {
	const crossHostURL = "https://example.com/OWNER/REPO/issues/9"

	// Each case exercises one relationship-bearing flag with a cross-host
	// URL. ResolveIssueRef should short-circuit before any GraphQL request,
	// and the per-issue failure must surface to stderr.
	tests := []struct {
		name  string
		input *EditOptions
	}{
		{
			name:  "set parent",
			input: &EditOptions{Parent: crossHostURL},
		},
		{
			name:  "add sub-issue",
			input: &EditOptions{AddSubIssues: []string{crossHostURL}},
		},
		{
			name:  "remove sub-issue",
			input: &EditOptions{RemoveSubIssues: []string{crossHostURL}},
		},
		{
			name:  "add blocked-by",
			input: &EditOptions{AddBlockedBy: []string{crossHostURL}},
		},
		{
			name:  "remove blocked-by",
			input: &EditOptions{RemoveBlockedBy: []string{crossHostURL}},
		},
		{
			name:  "add blocking",
			input: &EditOptions{AddBlocking: []string{crossHostURL}},
		},
		{
			name:  "remove blocking",
			input: &EditOptions{RemoveBlocking: []string{crossHostURL}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			mockIssueGet(t, reg)
			if tt.input.Parent != "" || tt.input.RemoveParent {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/issues/123/parent"),
					httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
				)
			}
			// No REST issue resolution stub on purpose: the cross-host guard must
			// short-circuit before any resolution request goes out.

			tt.input.Detector = &fd.EnabledDetectorMock{}
			tt.input.IssueNumbers = []int{123}
			tt.input.Interactive = false
			tt.input.FetchOptions = func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
				return nil
			}
			tt.input.IO = ios
			tt.input.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.input.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}

			err := editRun(tt.input)
			require.Error(t, err)
			assert.Regexp(t, `belongs to a different host \(example\.com\) than the current repository \(github\.com\)`, stderr.String())
		})
	}
}
