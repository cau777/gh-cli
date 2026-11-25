package readdir

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsonfieldstest"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFields(t *testing.T) {
	jsonfieldstest.ExpectCommandToSupportJSONFields(t, NewCmdReadDir, []string{
		"name",
		"path",
		"nameRaw",
		"pathRaw",
		"type",
		"gitType",
		"mode",
		"modeOctal",
		"gitSHA",
		"size",
		"submodule",
	})
}

func Test_entryTypeFromMode(t *testing.T) {
	tests := []struct {
		mode int
		want string
	}{
		{0o040000, "dir"},
		{0o120000, "symlink"},
		{0o160000, "submodule"},
		{0o100644, "file"},
		{0o100755, "file"},
		{0o100600, "file"},
		{0, "unknown"},
		{0o020000, "unknown"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, entryTypeFromMode(tt.mode), "mode %o", tt.mode)
	}
}

func Test_dirEntry_isExecutable(t *testing.T) {
	tests := []struct {
		name  string
		entry dirEntry
		want  bool
	}{
		{name: "executable file", entry: dirEntry{Type: "file", Mode: 0o100755}, want: true},
		{name: "regular file", entry: dirEntry{Type: "file", Mode: 0o100644}, want: false},
		{name: "directory", entry: dirEntry{Type: "dir", Mode: 0o040000}, want: false},
		{name: "symlink", entry: dirEntry{Type: "symlink", Mode: 0o120000}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.entry.isExecutable())
		})
	}
}

func Test_dirEntry_modeOctal(t *testing.T) {
	tests := []struct {
		name  string
		entry dirEntry
		want  string
	}{
		{name: "executable file", entry: dirEntry{Mode: 0o100755}, want: "100755"},
		{name: "regular file", entry: dirEntry{Mode: 0o100644}, want: "100644"},
		{name: "directory", entry: dirEntry{Mode: 0o040000}, want: "040000"},
		{name: "submodule", entry: dirEntry{Mode: 0o160000}, want: "160000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.entry.modeOctal())
		})
	}
}

func Test_NewCmdReadDir(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantOpts ReadDirOptions
		wantErr  string
	}{
		{
			name: "no args lists root",
			args: "",
			wantOpts: ReadDirOptions{
				Path: "",
			},
		},
		{
			name: "path argument",
			args: "pkg/cmd",
			wantOpts: ReadDirOptions{
				Path: "pkg/cmd",
			},
		},
		{
			name: "ref flag",
			args: "docs --ref v1.2.3",
			wantOpts: ReadDirOptions{
				Path: "docs",
				Ref:  "v1.2.3",
			},
		},
		{
			name:    "too many arguments",
			args:    "a b",
			wantErr: "accepts at most 1 arg(s), received 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var gotOpts *ReadDirOptions
			cmd := NewCmdReadDir(f, func(opts *ReadDirOptions) error {
				gotOpts = opts
				return nil
			})

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.Path, gotOpts.Path)
			assert.Equal(t, tt.wantOpts.Ref, gotOpts.Ref)
		})
	}
}

func Test_readDirRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       ReadDirOptions
		httpStubs  func(*httpmock.Registry)
		jsonFields []string
		wantOut    string
		wantStderr string
		wantErrMsg string
	}{
		{
			name: "base repo resolution error",
			opts: ReadDirOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return nil, errors.New("some error")
				},
			},
			wantErrMsg: "some error. Run this command from within a git repository, or use the `--repo` flag to specify one",
		},
		{
			name: "root listing (tty)",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents"),
					httpmock.StringResponse(compactJSON(`
						[
							{"name":".github","path":".github","type":"dir","sha":"oid-github","size":0},
							{"name":"README.md","path":"README.md","type":"file","sha":"oid-readme","size":2048},
							{"name":"latest","path":"latest","type":"symlink","sha":"oid-latest","size":18},
							{"name":"vendor","path":"vendor","type":"submodule","sha":"oid-vendor","size":0,"submodule_git_url":"https://github.com/OWNER/sub"}
						]`)),
				)
			},
			wantOut: heredoc.Doc(`
				Showing 4 entries in OWNER/REPO

				TYPE       NAME       SIZE
				dir        .github    -
				file       README.md  2.0 KB
				symlink    latest     18 B
				submodule  vendor     -
			`),
		},
		{
			name: "single entry uses singular noun (tty)",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents"),
					httpmock.StringResponse(`[{"name":"only.txt","path":"only.txt","type":"file","sha":"oid-only","size":3}]`),
				)
			},
			wantOut: heredoc.Doc(`
				Showing 1 entry in OWNER/REPO

				TYPE  NAME      SIZE
				file  only.txt  3 B
			`),
		},
		{
			name: "subdir listing header includes path (tty)",
			tty:  true,
			opts: ReadDirOptions{
				Path: "foo/bar",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/foo/bar"),
					httpmock.StringResponse(`[{"name":"baz.txt","path":"foo/bar/baz.txt","type":"file","sha":"oid-baz","size":10}]`),
				)
			},
			wantOut: heredoc.Doc(`
				Showing 1 entry in OWNER/REPO/foo/bar

				TYPE  NAME     SIZE
				file  baz.txt  10 B
			`),
		},
		{
			name: "ref is used in query parameter",
			opts: ReadDirOptions{
				Ref:  "v1.2.3",
				Path: "docs",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/contents/docs", map[string][]string{"ref": {"v1.2.3"}}),
					httpmock.StringResponse(`[{"name":"guide.md","path":"docs/guide.md","type":"file","sha":"oid-guide","size":1}]`),
				)
			},
			wantOut: "file\tguide.md\t100644\t1\n",
		},
		{
			name: "non-tty output is tab separated with octal mode and raw size",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents"),
					httpmock.StringResponse(compactJSON(`
						[
							{"name":".github","path":".github","type":"dir","sha":"oid-github","size":0},
							{"name":"README.md","path":"README.md","type":"file","sha":"oid-readme","size":512},
							{"name":"latest","path":"latest","type":"symlink","sha":"oid-latest","size":18}
						]`)),
				)
			},
			wantOut: "dir\t.github\t040000\t0\n" +
				"file\tREADME.md\t100644\t512\n" +
				"symlink\tlatest\t120000\t18\n",
		},
		{
			name: "json selects per-entry fields",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents"),
					httpmock.StringResponse(compactJSON(`
						[
							{"name":"a.txt","path":"a.txt","type":"file","sha":"oid-a","size":5},
							{"name":"docs","path":"docs","type":"dir","sha":"oid-docs","size":0}
						]`)),
				)
			},
			jsonFields: []string{"name", "type", "size", "gitType", "mode", "modeOctal"},
			wantOut: compactJSON(`
				{
					"entries":[
						{"gitType":"blob","mode":33188,"modeOctal":"100644","name":"a.txt","size":5,"type":"file"},
						{"gitType":"tree","mode":16384,"modeOctal":"040000","name":"docs","size":0,"type":"dir"}
					],
					"gitSHA":"",
					"id":""
				}`) + "\n",
		},
		{
			name: "json includes submodule details",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents"),
					httpmock.StringResponse(compactJSON(`
						[
							{"name":"vendor","path":"vendor","type":"submodule","sha":"abc123","size":0,"submodule_git_url":"https://github.com/OWNER/sub"}
						]`)),
				)
			},
			jsonFields: []string{"name", "type", "submodule"},
			wantOut: compactJSON(`
				{
					"entries":[
						{"name":"vendor","submodule":{"branch":null,"gitUrl":"https://github.com/OWNER/sub","subprojectCommitOid":"abc123"},"type":"submodule"}
					],
					"gitSHA":"",
					"id":""
				}`) + "\n",
		},
		{
			name: "empty directory warns and exits zero (tty)",
			tty:  true,
			opts: ReadDirOptions{
				Path: "empty",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/empty"),
					httpmock.StringResponse(`[]`),
				)
			},
			wantStderr: "No entries found in OWNER/REPO/empty\n",
		},
		{
			name: "empty root includes repo name in warning",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents"),
					httpmock.StringResponse(`[]`),
				)
			},
			wantStderr: "No entries found in OWNER/REPO\n",
		},
		{
			name: "path not found",
			opts: ReadDirOptions{
				Path: "missing",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/missing"),
					httpmock.StatusJSONResponse(404, map[string]string{"message": "Not Found"}),
				)
			},
			wantErrMsg: `could not find "missing" in OWNER/REPO`,
		},
		{
			name: "path or ref not found with ref",
			opts: ReadDirOptions{
				Path: "docs",
				Ref:  "nope",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/contents/docs", map[string][]string{"ref": {"nope"}}),
					httpmock.StatusJSONResponse(404, map[string]string{"message": "Not Found"}),
				)
			},
			wantErrMsg: `could not find "docs" at "nope" in OWNER/REPO (the path or ref may not exist)`,
		},
		{
			name: "http error surfaces cleanly",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents"),
					httpmock.StatusJSONResponse(500, map[string]string{"message": "Server Error"}),
				)
			},
			wantErrMsg: "HTTP 500: Server Error",
		},
		{
			name: "path points to a file",
			opts: ReadDirOptions{Path: "README.md"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/README.md"),
					httpmock.StringResponse(`{"name":"README.md","path":"README.md","type":"file","sha":"abc","size":10}`),
				)
			},
			wantErrMsg: "\"README.md\" is a file, not a directory; use `gh repo read-file` instead",
		},
		{
			name: "path points to a submodule",
			opts: ReadDirOptions{Path: "vendor-lib"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/vendor-lib"),
					httpmock.StringResponse(`{"name":"vendor-lib","path":"vendor-lib","type":"submodule","sha":"abc"}`),
				)
			},
			wantErrMsg: `"vendor-lib" is not a directory`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)

			opts := tt.opts
			opts.IO = ios
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			if opts.BaseRepo == nil {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				}
			}

			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}

			err := readDirRun(&opts)
			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func compactJSON(s string) string {
	var buf bytes.Buffer
	json.Compact(&buf, []byte(s))
	return buf.String()
}
