package develop

import (
	"net/http"
	"path/filepath"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DevelopOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)

	IssueNumber int
	Name        string
	BranchRepo  string
	BaseBranch  string
	Checkout    bool
	List        bool
	Worktree    string
}

func NewCmdDevelop(f *cmdutil.Factory, runF func(*DevelopOptions) error) *cobra.Command {
	opts := &DevelopOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
	}

	cmd := &cobra.Command{
		Use:   "develop {<number> | <url>}",
		Short: "Manage linked branches for an issue",
		Long: heredoc.Docf(`
			Manage linked branches for an issue.

			When using the %[1]s--base%[1]s flag, the new development branch will be created from the specified
			remote branch. The new branch will be configured as the base branch for pull requests created using
			%[1]sgh pr create%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# List branches for issue 123
			$ gh issue develop --list 123

			# List branches for issue 123 in repo cli/cli
			$ gh issue develop --list --repo cli/cli 123

			# Create a branch for issue 123 based on the my-feature branch
			$ gh issue develop 123 --base my-feature

			# Create a branch for issue 123 and check it out
			$ gh issue develop 123 --checkout

			# Create a branch for issue 123 and check it out in a worktree
			$ gh issue develop 123 --checkout --worktree /path/to/wt-feature

			# Create a branch in repo monalisa/cli for issue 123 in repo cli/cli
			$ gh issue develop 123 --repo cli/cli --branch-repo monalisa/cli
		`),
		Args: cmdutil.ExactArgs(1, "issue number or url is required"),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// This is all a hack to not break the deprecated issue-repo flag.
			// It will be removed in the near future and this hack can be removed at the same time.
			flags := cmd.Flags()
			if flags.Changed("issue-repo") {
				if flags.Changed("repo") {
					if flags.Changed("branch-repo") {
						return cmdutil.FlagErrorf("specify only `--repo` and `--branch-repo`")
					}
					branchRepo, _ := flags.GetString("repo")
					_ = flags.Set("branch-repo", branchRepo)
				}
				repo, _ := flags.GetString("issue-repo")
				_ = flags.Set("repo", repo)
			}
			if cmd.Parent() != nil {
				return cmd.Parent().PersistentPreRunE(cmd, args)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("worktree") && opts.Worktree == "" {
				return cmdutil.FlagErrorf("--worktree cannot be blank")
			}

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

			if err := cmdutil.MutuallyExclusive("specify only one of `--list` or `--branch-repo`", opts.List, opts.BranchRepo != ""); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--list` or `--base`", opts.List, opts.BaseBranch != ""); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--list` or `--checkout`", opts.List, opts.Checkout); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--list` or `--name`", opts.List, opts.Name != ""); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--list` or `--worktree`", opts.List, opts.Worktree != ""); err != nil {
				return err
			}
			if opts.Worktree != "" && !opts.Checkout {
				return cmdutil.FlagErrorf("--worktree requires --checkout")
			}
			if opts.Worktree != "" {
				opts.Worktree, err = filepath.Abs(opts.Worktree)
				if err != nil {
					return err
				}
			}
			if runF != nil {
				return runF(opts)
			}
			return developRun(opts)
		},
	}

	fl := cmd.Flags()
	fl.StringVar(&opts.BranchRepo, "branch-repo", "", "Name or URL of the repository where you want to create your new branch")
	fl.StringVarP(&opts.BaseBranch, "base", "b", "", "Name of the remote branch you want to make your new branch from")
	fl.BoolVarP(&opts.Checkout, "checkout", "c", false, "Checkout the branch after creating it")
	fl.BoolVarP(&opts.List, "list", "l", false, "List linked branches for the issue")
	fl.StringVarP(&opts.Name, "name", "n", "", "Name of the branch to create")
	fl.StringVar(&opts.Worktree, "worktree", "", "Check out the branch into a worktree at the given `path`")

	var issueRepoSelector string
	fl.StringVarP(&issueRepoSelector, "issue-repo", "i", "", "Name or URL of the issue's repository")
	_ = cmd.Flags().MarkDeprecated("issue-repo", "use `--repo` instead")

	return cmd
}

func developRun(opts *DevelopOptions) error {
	return api.NotSupportedError("linked issue branches (gh issue develop) are not supported with REST")
}
