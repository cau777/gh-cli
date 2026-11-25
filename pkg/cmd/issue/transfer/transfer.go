package transfer

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type TransferOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	IssueNumber      int
	DestRepoSelector string
}

func NewCmdTransfer(f *cmdutil.Factory, runF func(*TransferOptions) error) *cobra.Command {
	opts := TransferOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "transfer {<number> | <url>} <destination-repo>",
		Short: "Transfer issue to another repository",
		Args:  cmdutil.ExactArgs(2, "issue and destination repository are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			issueNumber, baseRepo, err := shared.ParseIssueFromArg(args[0])
			if err != nil {
				return err
			}

			// If the args provided the base repo then use that directly.
			if baseRepo, present := baseRepo.Value(); present {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return baseRepo, nil
				}
			} else {
				// support `-R, --repo` override
				opts.BaseRepo = f.BaseRepo
			}

			opts.IssueNumber = issueNumber

			opts.DestRepoSelector = args[1]

			if runF != nil {
				return runF(&opts)
			}

			return transferRun(&opts)
		},
	}

	return cmd
}

func transferRun(opts *TransferOptions) error {
	return api.NotSupportedError("transferring an issue is not supported with REST")
}
