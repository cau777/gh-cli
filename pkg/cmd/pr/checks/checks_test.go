package checks

import (
	"bytes"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdChecks(t *testing.T) {
	tests := []struct {
		name       string
		cli        string
		wants      ChecksOptions
		wantsError string
	}{
		{
			name: "no arguments",
			cli:  "",
			wants: ChecksOptions{
				Interval: time.Duration(10000000000),
			},
		},
		{
			name: "pr argument",
			cli:  "1234",
			wants: ChecksOptions{
				SelectorArg: "1234",
				Interval:    time.Duration(10000000000),
			},
		},
		{
			name: "watch flag",
			cli:  "--watch",
			wants: ChecksOptions{
				Watch:    true,
				Interval: time.Duration(10000000000),
			},
		},
		{
			name: "watch flag and interval flag",
			cli:  "--watch --interval 5",
			wants: ChecksOptions{
				Watch:    true,
				Interval: time.Duration(5000000000),
			},
		},
		{
			name:       "interval flag without watch flag",
			cli:        "--interval 5",
			wantsError: "cannot use `--interval` flag without `--watch` flag",
		},
		{
			name: "watch with fail-fast flag",
			cli:  "--watch --fail-fast",
			wants: ChecksOptions{
				Watch:    true,
				FailFast: true,
				Interval: time.Duration(10000000000),
			},
		},
		{
			name:       "fail-fast flag without watch flag",
			cli:        "--fail-fast",
			wantsError: "cannot use `--fail-fast` flag without `--watch` flag",
		},
		{
			name:       "watch with json flag",
			cli:        "--watch --json workflow",
			wantsError: "cannot use `--watch` with `--json` flag",
		},
		{
			name: "required flag",
			cli:  "--required",
			wants: ChecksOptions{
				Required: true,
				Interval: time.Duration(10000000000),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ChecksOptions
			cmd := NewCmdChecks(f, func(opts *ChecksOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsError != "" {
				assert.EqualError(t, err, tt.wantsError)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wants.SelectorArg, gotOpts.SelectorArg)
			assert.Equal(t, tt.wants.Watch, gotOpts.Watch)
			assert.Equal(t, tt.wants.Interval, gotOpts.Interval)
			assert.Equal(t, tt.wants.Required, gotOpts.Required)
			assert.Equal(t, tt.wants.FailFast, gotOpts.FailFast)
		})
	}
}

func Test_checksRun(t *testing.T) {
	tests := []struct {
		name            string
		tty             bool
		watch           bool
		failFast        bool
		required        bool
		disableDetector bool
		pr              *api.PullRequest
		httpStubs       func(*httpmock.Registry)
		wantOut         string
		wantErr         string
	}{
		{
			name: "no commits tty",
			tty:  true,
			pr:   &api.PullRequest{Number: 123},
			wantOut: "",
			wantErr: "no commit found on the pull request",
		},
		{
			name: "no checks tty",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, `[]`, `[]`)
			},
			wantOut: "",
			wantErr: "no checks reported on the 'trunk' branch",
		},
		{
			name: "some failing tty",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, someFailingRunsJSON, `[]`)
			},
			wantOut: heredoc.Doc(`
				Some checks were not successful
				0 cancelled, 1 failing, 1 successful, 0 skipped, and 1 pending checks

				   NAME        DESCRIPTION  ELAPSED  URL
				X  sad tests                1m26s    sweet link
				✓  cool tests               1m26s    sweet link
				*  slow tests               1m26s    sweet link
			`),
			wantErr: "SilentError",
		},
		{
			name: "some cancelled tty",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, someCancelledRunsJSON, `[]`)
			},
			wantOut: heredoc.Doc(`
				Some checks were cancelled
				1 cancelled, 0 failing, 2 successful, 0 skipped, and 0 pending checks

				   NAME           DESCRIPTION  ELAPSED  URL
				✓  cool tests                  1m26s    sweet link
				-  sad tests                   1m26s    sweet link
				✓  awesome tests               1m26s    sweet link
			`),
			wantErr: "",
		},
		{
			name: "some pending tty",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, somePendingRunsJSON, `[]`)
			},
			wantOut: heredoc.Doc(`
				Some checks are still pending
				1 cancelled, 0 failing, 2 successful, 0 skipped, and 1 pending checks

				   NAME        DESCRIPTION  ELAPSED  URL
				✓  cool tests               1m26s    sweet link
				✓  rad tests                1m26s    sweet link
				*  slow tests               1m26s    sweet link
				-  sad tests                1m26s    sweet link
			`),
			wantErr: "PendingError",
		},
		{
			name: "all passing tty",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, allPassingRunsJSON, `[]`)
			},
			wantOut: heredoc.Doc(`
				All checks were successful
				0 cancelled, 0 failing, 3 successful, 0 skipped, and 0 pending checks

				   NAME           DESCRIPTION  ELAPSED  URL
				✓  awesome tests               1m26s    sweet link
				✓  cool tests                  1m26s    sweet link
				✓  rad tests                   1m26s    sweet link
			`),
			wantErr: "",
		},
		{
			name:  "watch all passing tty",
			tty:   true,
			watch: true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, allPassingRunsJSON, `[]`)
			},
			wantOut: heredoc.Docf(`
				%[1]s[?1049hAll checks were successful
				0 cancelled, 0 failing, 3 successful, 0 skipped, and 0 pending checks

				   NAME           DESCRIPTION  ELAPSED  URL
				✓  awesome tests               1m26s    sweet link
				✓  cool tests                  1m26s    sweet link
				✓  rad tests                   1m26s    sweet link
				%[1]s[?1049lAll checks were successful
				0 cancelled, 0 failing, 3 successful, 0 skipped, and 0 pending checks

				   NAME           DESCRIPTION  ELAPSED  URL
				✓  awesome tests               1m26s    sweet link
				✓  cool tests                  1m26s    sweet link
				✓  rad tests                   1m26s    sweet link
			`, "\x1b"),
			wantErr: "",
		},
		{
			name:     "watch some failing with fail fast tty",
			tty:      true,
			watch:    true,
			failFast: true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, someFailingRunsJSON, `[]`)
			},
			wantOut: heredoc.Docf(`
				%[1]s[?1049h%[1]s[0;0H%[1]s[JRefreshing checks status every 0 seconds. Press Ctrl+C to quit.

				Some checks were not successful
				0 cancelled, 1 failing, 1 successful, 0 skipped, and 1 pending checks

				   NAME        DESCRIPTION  ELAPSED  URL
				X  sad tests                1m26s    sweet link
				✓  cool tests               1m26s    sweet link
				*  slow tests               1m26s    sweet link
				%[1]s[?1049lSome checks were not successful
				0 cancelled, 1 failing, 1 successful, 0 skipped, and 1 pending checks

				   NAME        DESCRIPTION  ELAPSED  URL
				X  sad tests                1m26s    sweet link
				✓  cool tests               1m26s    sweet link
				*  slow tests               1m26s    sweet link
			`, "\x1b"),
			wantErr: "SilentError",
		},
		{
			name: "with statuses tty",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, twoPassingRunsJSON, `[{"context":"a status","state":"failure","target_url":"sweet link"}]`)
			},
			wantOut: heredoc.Doc(`
				Some checks were not successful
				0 cancelled, 1 failing, 2 successful, 0 skipped, and 0 pending checks

				   NAME        DESCRIPTION  ELAPSED  URL
				X  a status                          sweet link
				✓  cool tests               1m26s    sweet link
				✓  rad tests                1m26s    sweet link
			`),
			wantErr: "SilentError",
		},
		{
			name:    "no commits",
			pr:      &api.PullRequest{Number: 123},
			wantOut: "",
			wantErr: "no commit found on the pull request",
		},
		{
			name: "no checks",
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, `[]`, `[]`)
			},
			wantOut: "",
			wantErr: "no checks reported on the 'trunk' branch",
		},
		{
			name: "some failing",
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, someFailingRunsJSON, `[]`)
			},
			wantOut: "sad tests\tfail\t1m26s\tsweet link\t\ncool tests\tpass\t1m26s\tsweet link\t\nslow tests\tpending\t1m26s\tsweet link\t\n",
			wantErr: "SilentError",
		},
		{
			name: "some pending",
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, somePendingRunsJSON, `[]`)
			},
			wantOut: "cool tests\tpass\t1m26s\tsweet link\t\nrad tests\tpass\t1m26s\tsweet link\t\nslow tests\tpending\t1m26s\tsweet link\t\nsad tests\tfail\t1m26s\tsweet link\t\n",
			wantErr: "PendingError",
		},
		{
			name: "all passing",
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, allPassingRunsJSON, `[]`)
			},
			wantOut: "awesome tests\tpass\t1m26s\tsweet link\t\ncool tests\tpass\t1m26s\tsweet link\t\nrad tests\tpass\t1m26s\tsweet link\t\n",
			wantErr: "",
		},
		{
			name: "with statuses",
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, twoPassingRunsJSON, `[{"context":"a status","state":"failure","target_url":"sweet link"}]`)
			},
			wantOut: "a status\tfail\t0\tsweet link\t\ncool tests\tpass\t1m26s\tsweet link\t\nrad tests\tpass\t1m26s\tsweet link\t\n",
			wantErr: "SilentError",
		},
		{
			name: "some skipped tty",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, someSkippingRunsJSON, `[]`)
			},
			wantOut: heredoc.Doc(`
				All checks were successful
				0 cancelled, 0 failing, 1 successful, 2 skipped, and 0 pending checks

				   NAME        DESCRIPTION  ELAPSED  URL
				✓  cool tests               1m26s    sweet link
				-  rad tests                1m26s    sweet link
				-  skip tests               1m26s    sweet link
			`),
			wantErr: "",
		},
		{
			name: "some skipped",
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, someSkippingRunsJSON, `[]`)
			},
			wantOut: "cool tests\tpass\t1m26s\tsweet link\t\nrad tests\tskipping\t1m26s\tsweet link\t\nskip tests\tskipping\t1m26s\tsweet link\t\n",
			wantErr: "",
		},
		{
			name:     "only required tty",
			tty:      true,
			required: true,
			wantOut:  "",
			wantErr:  "not all features are supported: filtering by --required checks is not supported with REST",
		},
		{
			name:     "only required",
			required: true,
			wantOut:  "",
			wantErr:  "not all features are supported: filtering by --required checks is not supported with REST",
		},
		{
			name: "descriptions tty",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, `[]`, withDescriptionsStatusesJSON)
			},
			wantOut: heredoc.Doc(`
				All checks were successful
				0 cancelled, 0 failing, 3 successful, 0 skipped, and 0 pending checks

				   NAME           DESCRIPTION          ELAPSED  URL
				✓  awesome tests  awesome description           sweet link
				✓  cool tests     cool description              sweet link
				✓  rad tests      rad description               sweet link
			`),
			wantErr: "",
		},
		{
			name: "descriptions",
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, `[]`, withDescriptionsStatusesJSON)
			},
			wantOut: "awesome tests\tpass\t0\tsweet link\tawesome description\ncool tests\tpass\t0\tsweet link\tcool description\nrad tests\tpass\t0\tsweet link\trad description\n",
			wantErr: "",
		},
		{
			name: "events tty",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, withEventsRunsJSON, `[]`)
				stubWorkflowRuns(reg)
			},
			wantOut: heredoc.Doc(`
				All checks were successful
				0 cancelled, 0 failing, 2 successful, 0 skipped, and 0 pending checks

				   NAME                             DESCRIPTION  ELAPSED  URL
				✓  tests/cool tests (pull_request)               1m26s    sweet link
				✓  tests/cool tests (push)                       1m26s    sweet link
			`),
			wantErr: "",
		},
		{
			name:            "events not supported tty",
			tty:             true,
			disableDetector: true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, withEventsRunsJSON, `[]`)
				stubWorkflowRuns(reg)
			},
			wantOut: heredoc.Doc(`
				All checks were successful
				0 cancelled, 0 failing, 1 successful, 0 skipped, and 0 pending checks

				   NAME              DESCRIPTION  ELAPSED  URL
				✓  tests/cool tests               1m26s    sweet link
			`),
			wantErr: "",
		},
		{
			name: "events",
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, withEventsRunsJSON, `[]`)
				stubWorkflowRuns(reg)
			},
			wantOut: "cool tests\tpass\t1m26s\tsweet link\t\ncool tests\tpass\t1m26s\tsweet link\t\n",
			wantErr: "",
		},
		{
			name:            "events not supported",
			disableDetector: true,
			httpStubs: func(reg *httpmock.Registry) {
				stubPRChecks(reg, withEventsRunsJSON, `[]`)
				stubWorkflowRuns(reg)
			},
			wantOut: "cool tests\tpass\t1m26s\tsweet link\t\n",
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetAlternateScreenBufferEnabled(tt.tty)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			var detector fd.Detector
			detector = &fd.EnabledDetectorMock{}
			if tt.disableDetector {
				detector = &fd.DisabledDetectorMock{}
			}

			response := tt.pr
			if response == nil {
				response = &api.PullRequest{Number: 123, HeadRefName: "trunk"}
			}

			opts := &ChecksOptions{
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				IO:          ios,
				SelectorArg: "123",
				Finder:      shared.NewMockFinder("123", response, ghrepo.New("OWNER", "REPO")),
				Detector:    detector,
				Watch:       tt.watch,
				FailFast:    tt.failFast,
				Required:    tt.required,
			}

			err := checksRun(opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				if tt.required {
					assert.True(t, errors.Is(err, api.ErrNotSupported))
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}

func TestChecksRun_web(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		wantStderr string
		wantStdout string
		wantBrowse string
	}{
		{
			name:       "tty",
			isTTY:      true,
			wantStderr: "Opening https://github.com/OWNER/REPO/pull/123/checks in your browser.\n",
			wantStdout: "",
			wantBrowse: "https://github.com/OWNER/REPO/pull/123/checks",
		},
		{
			name:       "nontty",
			isTTY:      false,
			wantStderr: "",
			wantStdout: "",
			wantBrowse: "https://github.com/OWNER/REPO/pull/123/checks",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			browser := &browser.Stub{}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tc.isTTY)
			ios.SetStdinTTY(tc.isTTY)
			ios.SetStderrTTY(tc.isTTY)

			_, teardown := run.Stub()
			defer teardown(t)

			err := checksRunWebMode(&ChecksOptions{
				IO:          ios,
				Browser:     browser,
				WebMode:     true,
				SelectorArg: "123",
				Finder:      shared.NewMockFinder("123", &api.PullRequest{Number: 123}, ghrepo.New("OWNER", "REPO")),
			})
			assert.NoError(t, err)
			assert.Equal(t, tc.wantStdout, stdout.String())
			assert.Equal(t, tc.wantStderr, stderr.String())
			browser.Verify(t, tc.wantBrowse)
		})
	}
}

func TestEliminateDuplicates(t *testing.T) {
	tests := []struct {
		name          string
		checkContexts []api.CheckContext
		want          []api.CheckContext
	}{
		{
			name: "duplicate CheckRun (lint)",
			checkContexts: []api.CheckContext{
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/1",
				},
				{
					TypeName:    "CheckRun",
					Name:        "lint",
					Status:      "COMPLETED",
					Conclusion:  "FAILURE",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/2",
				},
				{
					TypeName:    "CheckRun",
					Name:        "lint",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 2, 2, 2, 2, 2, 2, time.UTC),
					CompletedAt: time.Date(2022, 2, 2, 2, 2, 2, 2, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/3",
				},
			},
			want: []api.CheckContext{
				{
					TypeName:    "CheckRun",
					Name:        "lint",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 2, 2, 2, 2, 2, 2, time.UTC),
					CompletedAt: time.Date(2022, 2, 2, 2, 2, 2, 2, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/3",
				},
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/1",
				},
			},
		},
		{
			name: "duplicate StatusContext (Windows GPU)",
			checkContexts: []api.CheckContext{
				{
					TypeName:    "StatusContext",
					Name:        "",
					Context:     "Windows GPU",
					State:       "FAILURE",
					Status:      "",
					Conclusion:  "",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "",
					TargetURL:   "https://github.com/cli/cli/2",
				},
				{
					TypeName:    "StatusContext",
					Name:        "",
					Context:     "Windows GPU",
					State:       "SUCCESS",
					Status:      "",
					Conclusion:  "",
					StartedAt:   time.Date(2022, 2, 2, 2, 2, 2, 2, time.UTC),
					CompletedAt: time.Date(2022, 2, 2, 2, 2, 2, 2, time.UTC),
					DetailsURL:  "",
					TargetURL:   "https://github.com/cli/cli/3",
				},
				{
					TypeName:    "StatusContext",
					Name:        "",
					Context:     "Linux GPU",
					State:       "SUCCESS",
					Status:      "",
					Conclusion:  "",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "",
					TargetURL:   "https://github.com/cli/cli/1",
				},
			},
			want: []api.CheckContext{
				{
					TypeName:    "StatusContext",
					Name:        "",
					Context:     "Windows GPU",
					State:       "SUCCESS",
					Status:      "",
					Conclusion:  "",
					StartedAt:   time.Date(2022, 2, 2, 2, 2, 2, 2, time.UTC),
					CompletedAt: time.Date(2022, 2, 2, 2, 2, 2, 2, time.UTC),
					DetailsURL:  "",
					TargetURL:   "https://github.com/cli/cli/3",
				},
				{
					TypeName:    "StatusContext",
					Name:        "",
					Context:     "Linux GPU",
					State:       "SUCCESS",
					Status:      "",
					Conclusion:  "",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "",
					TargetURL:   "https://github.com/cli/cli/1",
				},
			},
		},
		{
			name: "unique CheckContext",
			checkContexts: []api.CheckContext{
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/1",
				},
				{
					TypeName:    "StatusContext",
					Name:        "",
					Context:     "Windows GPU",
					State:       "SUCCESS",
					Status:      "",
					Conclusion:  "",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "",
					TargetURL:   "https://github.com/cli/cli/2",
				},
				{
					TypeName:    "StatusContext",
					Name:        "",
					Context:     "Linux GPU",
					State:       "SUCCESS",
					Status:      "",
					Conclusion:  "",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "",
					TargetURL:   "https://github.com/cli/cli/3",
				},
			},
			want: []api.CheckContext{
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/1",
				},
				{
					TypeName:    "StatusContext",
					Name:        "",
					Context:     "Windows GPU",
					State:       "SUCCESS",
					Status:      "",
					Conclusion:  "",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "",
					TargetURL:   "https://github.com/cli/cli/2",
				},
				{
					TypeName:    "StatusContext",
					Name:        "",
					Context:     "Linux GPU",
					State:       "SUCCESS",
					Status:      "",
					Conclusion:  "",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "",
					TargetURL:   "https://github.com/cli/cli/3",
				},
			},
		},
		{
			name: "unique workflow name",
			checkContexts: []api.CheckContext{
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/1",
					CheckSuite: api.CheckSuite{
						WorkflowRun: api.WorkflowRun{
							Event: "push",
							Workflow: api.Workflow{
								Name: "some builds",
							},
						},
					},
				},
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/2",
					CheckSuite: api.CheckSuite{
						WorkflowRun: api.WorkflowRun{
							Event: "push",
							Workflow: api.Workflow{
								Name: "some other builds",
							},
						},
					},
				},
			},
			want: []api.CheckContext{
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/1",
					CheckSuite: api.CheckSuite{
						WorkflowRun: api.WorkflowRun{
							Event: "push",
							Workflow: api.Workflow{
								Name: "some builds",
							},
						},
					},
				},
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/2",
					CheckSuite: api.CheckSuite{
						WorkflowRun: api.WorkflowRun{
							Event: "push",
							Workflow: api.Workflow{
								Name: "some other builds",
							},
						},
					},
				},
			},
		},
		{
			name: "unique workflow run event",
			checkContexts: []api.CheckContext{
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/1",
					CheckSuite: api.CheckSuite{
						WorkflowRun: api.WorkflowRun{
							Event: "push",
							Workflow: api.Workflow{
								Name: "builds",
							},
						},
					},
				},
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/2",
					CheckSuite: api.CheckSuite{
						WorkflowRun: api.WorkflowRun{
							Event: "pull_request",
							Workflow: api.Workflow{
								Name: "builds",
							},
						},
					},
				},
			},
			want: []api.CheckContext{
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/1",
					CheckSuite: api.CheckSuite{
						WorkflowRun: api.WorkflowRun{
							Event: "push",
							Workflow: api.Workflow{
								Name: "builds",
							},
						},
					},
				},
				{
					TypeName:    "CheckRun",
					Name:        "build (ubuntu-latest)",
					Status:      "COMPLETED",
					Conclusion:  "SUCCESS",
					StartedAt:   time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					CompletedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
					DetailsURL:  "https://github.com/cli/cli/runs/2",
					CheckSuite: api.CheckSuite{
						WorkflowRun: api.WorkflowRun{
							Event: "pull_request",
							Workflow: api.Workflow{
								Name: "builds",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eliminateDuplicates(tt.checkContexts)
			if !reflect.DeepEqual(tt.want, got) {
				t.Errorf("got eliminateDuplicates %+v, want %+v\n", got, tt.want)
			}
		})
	}
}


func stubPRChecks(reg *httpmock.Registry, checkRunsJSON, statusesJSON string) {
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/commits/trunk/check-runs"),
		httpmock.StringResponse(`{"check_runs":`+checkRunsJSON+`}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/commits/trunk/status"),
		httpmock.StringResponse(`{"statuses":`+statusesJSON+`}`),
	)
}

func stubWorkflowRuns(reg *httpmock.Registry) {
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
		httpmock.StringResponse(`{"workflow_runs":[
			{"id":1,"name":"tests","event":"pull_request","check_suite_id":10},
			{"id":2,"name":"tests","event":"push","check_suite_id":20}
		]}`),
	)
}

const (
	allPassingRunsJSON = `[
		{"name":"cool tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"rad tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"awesome tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"}
	]`
	twoPassingRunsJSON = `[
		{"name":"cool tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"rad tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"}
	]`
	someFailingRunsJSON = `[
		{"name":"cool tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"sad tests","status":"completed","conclusion":"failure","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"slow tests","status":"in_progress","conclusion":"","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"}
	]`
	someCancelledRunsJSON = `[
		{"name":"cool tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"sad tests","status":"completed","conclusion":"cancelled","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"awesome tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"}
	]`
	somePendingRunsJSON = `[
		{"name":"cool tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"rad tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"slow tests","status":"in_progress","conclusion":"","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"sad tests","status":"completed","conclusion":"cancelled","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"}
	]`
	someSkippingRunsJSON = `[
		{"name":"cool tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"rad tests","status":"completed","conclusion":"skipped","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"},
		{"name":"skip tests","status":"completed","conclusion":"skipped","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link"}
	]`
	withDescriptionsStatusesJSON = `[
		{"context":"cool tests","description":"cool description","state":"success","target_url":"sweet link"},
		{"context":"rad tests","description":"rad description","state":"success","target_url":"sweet link"},
		{"context":"awesome tests","description":"awesome description","state":"success","target_url":"sweet link"}
	]`
	withEventsRunsJSON = `[
		{"name":"cool tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link","check_suite":{"id":10}},
		{"name":"cool tests","status":"completed","conclusion":"success","started_at":"2020-08-27T18:58:46Z","completed_at":"2020-08-27T19:00:12Z","details_url":"sweet link","check_suite":{"id":20}}
	]`
)
