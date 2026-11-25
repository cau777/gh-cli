package pin

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

type PinOptions struct {
	HttpClient  func() (*http.Client, error)
	Config      func() (gh.Config, error)
	IO          *iostreams.IOStreams
	BaseRepo    func() (ghrepo.Interface, error)
	IssueNumber int
}

func NewCmdPin(f *cmdutil.Factory, runF func(*PinOptions) error) *cobra.Command {
	opts := &PinOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "pin {<number> | <url>}",
		Short: "Pin an issue",
		Long: heredoc.Doc(`
			Pin an issue to a repository.

			The issue can be specified by issue number or URL.
		`),
		Example: heredoc.Doc(`
			# Pin an issue to the current repository
			$ gh issue pin 23

			# Pin an issue by URL
			$ gh issue pin https://github.com/owner/repo/issues/23

			# Pin an issue to specific repository
			$ gh issue pin 23 --repo owner/repo
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

			return pinRun(opts)
		},
	}

	return cmd
}

func pinRun(opts *PinOptions) error {
	return api.NotSupportedError("pinning an issue is not supported with REST")
}
