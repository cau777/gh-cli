package revert

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
	}

	cmd := NewCmdRevert(factory, nil)

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestPRRevert_missingArgument(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "MERGED",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	_, err := runCommand(http, true, "")
	assert.EqualError(t, err, "cannot revert pull request: number, url, or branch required")
}

func TestPRRevert_notSupported(t *testing.T) {
	tests := []struct {
		name string
		args string
	}{
		{
			name: "Revert by pull request number",
			args: "123",
		},
		{
			name: "Revert with flags",
			args: "123 --title 'Custom' --body 'Body' --draft",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			output, err := runCommand(http, true, tt.args)
			assert.ErrorIs(t, err, api.ErrNotSupported)
			assert.Equal(t, "", output.String())
			assert.Equal(t, "", output.Stderr())
		})
	}
}
