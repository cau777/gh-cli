package revert

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RevertOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Finder shared.PRFinder

	SelectorArg string

	Body    string
	BodySet bool
	Title   string
	IsDraft bool
}

func NewCmdRevert(f *cmdutil.Factory, runF func(*RevertOptions) error) *cobra.Command {
	opts := &RevertOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "revert {<number> | <url> | <branch>}",
		Short: "Revert a pull request",
		Args:  cmdutil.ExactArgs(1, "cannot revert pull request: number, url, or branch required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			bodyProvided := cmd.Flags().Changed("body")
			bodyFileProvided := bodyFile != ""

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--body` or `--body-file`",
				bodyProvided,
				bodyFileProvided,
			); err != nil {
				return err
			}

			if bodyProvided || bodyFileProvided {
				opts.BodySet = true
				if bodyFileProvided {
					b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
					if err != nil {
						return err
					}
					opts.Body = string(b)
				}
			}

			if runF != nil {
				return runF(opts)
			}
			return revertRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.IsDraft, "draft", "d", false, "Mark revert pull request as a draft")
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Title for the revert pull request")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Body for the revert pull request")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	return cmd
}

func revertRun(opts *RevertOptions) error {
	return api.NotSupportedError("reverting a pull request is not supported with REST")
}
