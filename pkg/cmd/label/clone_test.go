package label

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdClone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  cloneOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no argument",
			input:   "",
			wantErr: true,
			errMsg:  "cannot clone labels: source-repository argument required",
		},
		{
			name:  "source-repository argument",
			input: "OWNER/REPO",
			output: cloneOptions{
				SourceRepo: ghrepo.New("OWNER", "REPO"),
			},
		},
		{
			name:  "force flag",
			input: "OWNER/REPO --force",
			output: cloneOptions{
				SourceRepo: ghrepo.New("OWNER", "REPO"),
				Force:      true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}

			var gotOpts *cloneOptions
			cmd := newCmdClone(f, func(opts *cloneOptions) error {
				gotOpts = opts
				return nil
			})

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.SourceRepo, gotOpts.SourceRepo)
			assert.Equal(t, tt.output.Force, gotOpts.Force)
		})
	}
}

func TestCloneRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       *cloneOptions
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "clones all labels",
			tty:  true,
			opts: &cloneOptions{SourceRepo: ghrepo.New("cli", "cli")},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/labels"),
					httpmock.StringResponse(`[
						{
							"name": "bug",
							"color": "d73a4a",
							"description": "Something isn't working"
						},
						{
							"name": "docs",
							"color": "6cafc9"
						}
					]`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.StatusStringResponse(201, `
					{
						"name": "bug",
						"color": "d73a4a",
						"description": "Something isn't working"
					}`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.StatusStringResponse(201, `
					{
						"name": "docs",
						"color": "6cafc9"
					}`),
				)
			},
			wantStdout: "✓ Cloned 2 labels from cli/cli to OWNER/REPO\n",
		},
		{
			name: "clones one label",
			tty:  true,
			opts: &cloneOptions{SourceRepo: ghrepo.New("cli", "cli")},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/labels"),
					httpmock.StringResponse(`[
						{
							"name": "bug",
							"color": "d73a4a",
							"description": "Something isn't working"
						}
					]`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.StatusStringResponse(201, `
					{
						"name": "bug",
						"color": "d73a4a",
						"description": "Something isn't working"
					}`),
				)
			},
			wantStdout: "✓ Cloned 1 label from cli/cli to OWNER/REPO\n",
		},
		{
			name: "has no labels",
			tty:  true,
			opts: &cloneOptions{SourceRepo: ghrepo.New("cli", "cli")},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/labels"),
					httpmock.StringResponse(`[]`),
				)
			},
			wantStdout: "✓ Cloned 0 labels from cli/cli to OWNER/REPO\n",
		},
		{
			name: "clones some labels",
			tty:  true,
			opts: &cloneOptions{SourceRepo: ghrepo.New("cli", "cli")},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/labels"),
					httpmock.StringResponse(`[
						{
							"name": "bug",
							"color": "d73a4a",
							"description": "Something isn't working"
						},
						{
							"name": "docs",
							"color": "6cafc9"
						}
					]`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.StatusStringResponse(201, `
					{
						"name": "bug",
						"color": "d73a4a",
						"description": "Something isn't working"
					}`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422, `{"message":"Validation Failed","errors":[{"resource":"Label","code":"already_exists","field":"name"}]}`),
						"Content-Type",
						"application/json",
					),
				)
			},
			wantStdout: "! Cloned 1 label of 2 from cli/cli to OWNER/REPO\n",
		},
		{
			name: "clones no labels",
			tty:  true,
			opts: &cloneOptions{SourceRepo: ghrepo.New("cli", "cli")},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/labels"),
					httpmock.StringResponse(`[
						{
							"name": "bug",
							"color": "d73a4a",
							"description": "Something isn't working"
						},
						{
							"name": "docs",
							"color": "6cafc9"
						}
					]`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422, `{"message":"Validation Failed","errors":[{"resource":"Label","code":"already_exists","field":"name"}]}`),
						"Content-Type",
						"application/json",
					),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422, `{"message":"Validation Failed","errors":[{"resource":"Label","code":"already_exists","field":"name"}]}`),
						"Content-Type",
						"application/json",
					),
				)
			},
			wantStdout: "! Cloned 0 labels of 2 from cli/cli to OWNER/REPO\n",
		},
		{
			name: "overwrites all labels",
			tty:  true,
			opts: &cloneOptions{SourceRepo: ghrepo.New("cli", "cli"), Force: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/labels"),
					httpmock.StringResponse(`[
						{
							"name": "bug",
							"color": "d73a4a",
							"description": "Something isn't working"
						},
						{
							"name": "docs",
							"color": "6cafc9"
						}
					]`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422, `{"message":"Validation Failed","errors":[{"resource":"Label","code":"already_exists","field":"name"}]}`),
						"Content-Type",
						"application/json",
					),
				)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/labels/bug"),
					httpmock.StatusStringResponse(201, `
					{
						"color": "d73a4a",
						"description": "Something isn't working"
					}`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422, `{"message":"Validation Failed","errors":[{"resource":"Label","code":"already_exists","field":"name"}]}`),
						"Content-Type",
						"application/json",
					),
				)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/labels/docs"),
					httpmock.StatusStringResponse(201, `
					{
						"color": "6cafc9"
					}`),
				)
			},
			wantStdout: "✓ Cloned 2 labels from cli/cli to OWNER/REPO\n",
		},
		{
			name: "list error",
			tty:  true,
			opts: &cloneOptions{SourceRepo: ghrepo.New("cli", "invalid"), Force: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/cli/invalid/labels"),
					httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
				)
			},
			wantErr:    true,
			wantErrMsg: "HTTP 404 (https://api.github.com/repos/cli/invalid/labels?per_page=100)",
		},
		{
			name: "create error",
			tty:  true,
			opts: &cloneOptions{SourceRepo: ghrepo.New("cli", "cli"), Force: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/labels"),
					httpmock.StringResponse(`[
						{
							"name": "bug",
							"color": "d73a4a",
							"description": "Something isn't working"
						}
					]`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422, `{"message":"Validation Failed","errors":[{"resource":"Label","code":"invalid","field":"color"}]}`),
						"Content-Type",
						"application/json",
					),
				)
			},
			wantErr:    true,
			wantErrMsg: "HTTP 422: Validation Failed (https://api.github.com/repos/OWNER/REPO/labels)\nLabel.color is invalid",
		},
		{
			name: "clones pages of labels",
			tty:  true,
			opts: &cloneOptions{SourceRepo: ghrepo.New("cli", "cli"), Force: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/cli/cli/labels", map[string][]string{"per_page": {"100"}}),
					httpmock.WithHeader(
						httpmock.StringResponse(`[
							{
								"name": "bug",
								"color": "d73a4a",
								"description": "Something isn't working"
							}
						]`),
						"Link",
						`<https://api.github.com/repos/cli/cli/labels?page=2&per_page=100>; rel="next"`,
					),
				)
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/cli/cli/labels", map[string][]string{"page": {"2"}, "per_page": {"100"}}),
					httpmock.StringResponse(`[
						{
							"name": "docs",
							"color": "6cafc9"
						}
					]`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.StatusStringResponse(201, `
					{
						"name": "bug",
						"color": "d73a4a",
						"description": "Something isn't working"
					}`),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.StatusStringResponse(201, `
					{
						"name": "docs",
						"color": "6cafc9"
					}`),
				)
			},
			wantStdout: "✓ Cloned 2 labels from cli/cli to OWNER/REPO\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			io, _, stdout, _ := iostreams.Test()
			io.SetStdoutTTY(tt.tty)
			io.SetStdinTTY(tt.tty)
			io.SetStderrTTY(tt.tty)
			tt.opts.IO = io
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}

			err := cloneRun(tt.opts)

			if tt.wantErr {
				assert.EqualError(t, err, tt.wantErrMsg)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
