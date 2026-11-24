package ready

import (
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ReadyOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Finder shared.PRFinder

	SelectorArg string
	Undo        bool
}

func NewCmdReady(f *cmdutil.Factory, runF func(*ReadyOptions) error) *cobra.Command {
	opts := &ReadyOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "ready [<number> | <url> | <branch>]",
		Short: "Mark a pull request as ready for review",
		Long: heredoc.Docf(`
			Mark a pull request as ready for review.

			Without an argument, the pull request that belongs to the current branch
			is marked as ready.

			If supported by your plan, convert to draft with %[1]s--undo%[1]s
		`, "`"),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the --repo flag")
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return readyRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Undo, "undo", false, `Convert a pull request to "draft"`)
	return cmd
}

func readyRun(opts *ReadyOptions) error {
	if opts.Undo {
		return api.NotSupportedError("converting a pull request to draft is not supported with REST")
	}
	return api.NotSupportedError("marking a pull request ready for review is not supported with REST")
}
