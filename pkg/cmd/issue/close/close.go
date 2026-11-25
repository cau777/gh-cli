package close

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CloseOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	IssueNumber int
	Comment     string
	Reason      string
	DuplicateOf string
}

func NewCmdClose(f *cmdutil.Factory, runF func(*CloseOptions) error) *cobra.Command {
	opts := &CloseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "close {<number> | <url>}",
		Short: "Close issue",
		Example: heredoc.Doc(`
			# Close issue
			$ gh issue close 123

			# Close issue and add a closing comment
			$ gh issue close 123 --comment "Closing this issue"

			# Close issue as a duplicate of issue #456
			$ gh issue close 123 --duplicate-of 456

			# Close issue as not planned
			$ gh issue close 123 --reason "not planned"
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
			if opts.DuplicateOf != "" {
				if opts.Reason == "" {
					opts.Reason = "duplicate"
				} else if opts.Reason != "duplicate" {
					return cmdutil.FlagErrorf("`--duplicate-of` can only be used with `--reason duplicate`")
				}
			}

			if runF != nil {
				return runF(opts)
			}
			return closeRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "Leave a closing comment")
	cmdutil.StringEnumFlag(cmd, &opts.Reason, "reason", "r", "", []string{"completed", "not planned", "duplicate"}, "Reason for closing")
	cmd.Flags().StringVar(&opts.DuplicateOf, "duplicate-of", "", "Mark as duplicate of another issue by number or URL")

	return cmd
}

func closeRun(opts *CloseOptions) error {
	cs := opts.IO.ColorScheme()
	closeReason := opts.Reason
	if opts.DuplicateOf != "" {
		if closeReason == "" {
			closeReason = "duplicate"
		} else if closeReason != "duplicate" {
			return cmdutil.FlagErrorf("`--duplicate-of` can only be used with `--reason duplicate`")
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if opts.DuplicateOf != "" {
		return api.NotSupportedError("marking an issue as a duplicate of a specific issue is not supported with REST")
	}

	issue, err := shared.FindIssueOrPR(httpClient, baseRepo, opts.IssueNumber, []string{"id", "number", "title", "state"})
	if err != nil {
		return err
	}

	if issue.State == "CLOSED" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Issue %s#%d (%s) is already closed\n", cs.Yellow("!"), ghrepo.FullName(baseRepo), issue.Number, issue.Title)
		return nil
	}

	if opts.Comment != "" {
		commentOpts := &prShared.CommentableOptions{
			Body:       opts.Comment,
			HttpClient: opts.HttpClient,
			InputType:  prShared.InputTypeInline,
			Quiet:      true,
			RetrieveCommentable: func() (prShared.Commentable, ghrepo.Interface, error) {
				return issue, baseRepo, nil
			},
		}
		err := prShared.CommentableRun(commentOpts)
		if err != nil {
			return err
		}
	}

	err = apiClose(httpClient, baseRepo, issue, closeReason)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Closed issue %s#%d (%s)\n", cs.SuccessIconWithColor(cs.Red), ghrepo.FullName(baseRepo), issue.Number, issue.Title)

	return nil
}

func apiClose(httpClient *http.Client, repo ghrepo.Interface, issue *api.Issue, reason string) error {
	if issue.IsPullRequest() {
		return api.PullRequestClose(httpClient, repo, issue.Number)
	}

	switch reason {
	case "":
		// If no reason is specified do not set it.
	case "not planned":
		reason = "not_planned"
	case "duplicate":
		reason = "duplicate"
	default:
		reason = "completed"
	}

	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(issue.Number))
	if err != nil {
		return err
	}

	payload, err := json.Marshal(struct {
		State       string `json:"state"`
		StateReason string `json:"state_reason,omitempty"`
	}{
		State:       "closed",
		StateReason: reason,
	})
	if err != nil {
		return err
	}

	client := api.NewClientFromHTTP(httpClient)
	return client.REST(repo.RepoHost(), http.MethodPatch, p.String(), bytes.NewReader(payload), nil)
}
