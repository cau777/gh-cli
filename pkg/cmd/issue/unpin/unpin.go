package unpin

import (
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type UnpinOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	IssueNumber int
}

func NewCmdUnpin(f *cmdutil.Factory, runF func(*UnpinOptions) error) *cobra.Command {
	opts := &UnpinOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "unpin {<number> | <url>}",
		Short: "Unpin an issue",
		Long: heredoc.Doc(`
			Unpin an issue from a repository.

			The issue can be specified by issue number or URL.
		`),
		Example: heredoc.Doc(`
			# Unpin issue from the current repository
			$ gh issue unpin 23

			# Unpin issue by URL
			$ gh issue unpin https://github.com/owner/repo/issues/23

			# Unpin an issue from specific repository
			$ gh issue unpin 23 --repo owner/repo
		`),
		Args: cobra.ExactArgs(1),
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

			if runF != nil {
				return runF(opts)
			}

			return unpinRun(opts)
		},
	}

	return cmd
}

func unpinRun(opts *UnpinOptions) error {
	return api.NotSupportedError("unpinning an issue is not supported with REST")
}
