package unpin

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/argparsetest"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdUnpin(t *testing.T) {
	// Test shared parsing of issue number / URL.
	argparsetest.TestArgParsing(t, NewCmdUnpin)
}

func TestUnpinRun(t *testing.T) {
	tests := []struct {
		name      string
		tty       bool
		opts      *UnpinOptions
		httpStubs func(*httpmock.Registry)
		wantErr   error
	}{
		{
			name:    "unpin unsupported",
			tty:     true,
			opts:    &UnpinOptions{IssueNumber: 20},
			wantErr: api.ErrNotSupported,
		},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		ios, _, _, _ := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios

		tt.opts.Config = func() (gh.Config, error) {
			return config.NewMockConfig(), nil
		}

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		}

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := unpinRun(tt.opts)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
