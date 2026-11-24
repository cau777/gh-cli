package shared

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	o "github.com/cli/cli/v2/pkg/option"
	"golang.org/x/sync/errgroup"
)

var issueURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/(?:issues|pull)/(\d+)`)

func ParseIssuesFromArgs(args []string) ([]int, o.Option[ghrepo.Interface], error) {
	var repo o.Option[ghrepo.Interface]
	issueNumbers := make([]int, len(args))

	for i, arg := range args {
		// For each argument, parse the issue number and an optional repo
		issueNumber, issueRepo, err := ParseIssueFromArg(arg)
		if err != nil {
			return nil, o.None[ghrepo.Interface](), err
		}

		// if this is our first issue repo found, then we need to set it
		if repo.IsNone() {
			repo = issueRepo
		}

		// if there is an issue repo returned, then we need to check if it is the same as the previous one
		if issueRepo.IsSome() && repo.IsSome() {
			// Unwraps are safe because we've checked for presence above
			if !ghrepo.IsSame(repo.Unwrap(), issueRepo.Unwrap()) {
				return nil, o.None[ghrepo.Interface](), fmt.Errorf(
					"multiple issues must be in same repo: found %q, expected %q",
					ghrepo.FullName(issueRepo.Unwrap()),
					ghrepo.FullName(repo.Unwrap()),
				)
			}
		}

		// add the issue number to the list
		issueNumbers[i] = issueNumber
	}

	return issueNumbers, repo, nil
}

func ParseIssueFromArg(arg string) (int, o.Option[ghrepo.Interface], error) {
	issueLocator := tryParseIssueFromURL(arg)
	if issueLocator, present := issueLocator.Value(); present {
		return issueLocator.issueNumber, o.Some(issueLocator.repo), nil
	}

	issueNumber, err := strconv.Atoi(strings.TrimPrefix(arg, "#"))
	if err != nil {
		return 0, o.None[ghrepo.Interface](), fmt.Errorf("invalid issue format: %q", arg)
	}

	return issueNumber, o.None[ghrepo.Interface](), nil
}

type issueLocator struct {
	issueNumber int
	repo        ghrepo.Interface
}

// tryParseIssueFromURL tries to parse an issue number and repo from a URL.
func tryParseIssueFromURL(maybeURL string) o.Option[issueLocator] {
	u, err := url.Parse(maybeURL)
	if err != nil {
		return o.None[issueLocator]()
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return o.None[issueLocator]()
	}

	m := issueURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return o.None[issueLocator]()
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	issueNumber, _ := strconv.Atoi(m[3])
	return o.Some(issueLocator{
		issueNumber: issueNumber,
		repo:        repo,
	})
}

type PartialLoadError struct {
	error
}

// FindIssuesOrPRs loads 1 or more issues or pull requests with the specified fields. If some of the fields
// could not be fetched by GraphQL, this returns non-nil issues and a *PartialLoadError.
func FindIssuesOrPRs(httpClient *http.Client, repo ghrepo.Interface, issueNumbers []int, fields []string) ([]*api.Issue, error) {
	issuesChan := make(chan *api.Issue, len(issueNumbers))
	g := errgroup.Group{}
	for _, num := range issueNumbers {
		issueNumber := num
		g.Go(func() error {
			issue, err := FindIssueOrPR(httpClient, repo, issueNumber, fields)
			if err != nil {
				return err
			}

			issuesChan <- issue
			return nil
		})
	}

	err := g.Wait()
	close(issuesChan)

	if err != nil {
		return nil, err
	}

	issues := make([]*api.Issue, 0, len(issueNumbers))
	for issue := range issuesChan {
		issues = append(issues, issue)
	}

	return issues, nil
}

func FindIssueOrPR(httpClient *http.Client, repo ghrepo.Interface, number int, fields []string) (*api.Issue, error) {
	client := api.NewClientFromHTTP(httpClient)
	return api.SingleIssueREST(client, repo, number, fields)
}

// ResolveIssueRef parses an issue reference (number or URL) and returns its
// node ID. References that point at a different host than baseRepo are
// rejected because relationship mutations require IDs from the base host.
func ResolveIssueRef(client *api.Client, baseRepo ghrepo.Interface, ref string) (string, error) {
	number, repo, err := ParseIssueFromArg(ref)
	if err != nil {
		return "", err
	}

	targetRepo := baseRepo
	if r, ok := repo.Value(); ok {
		if r.RepoHost() != baseRepo.RepoHost() {
			return "", fmt.Errorf("issue reference %q belongs to a different host (%s) than the current repository (%s)", ref, r.RepoHost(), baseRepo.RepoHost())
		}
		targetRepo = r
	}

	return api.IssueNodeID(client, targetRepo, number)
}

// ResolveIssueTypeName resolves an issue type name to its node ID by
// fetching the repository's available types.
func ResolveIssueTypeName(client *api.Client, repo ghrepo.Interface, typeName string) (string, error) {
	issueTypes, err := api.RepoIssueTypes(client, repo)
	if err != nil {
		return "", err
	}

	typeNames := make([]string, len(issueTypes))
	for i, t := range issueTypes {
		typeNames[i] = t.Name
		if strings.EqualFold(t.Name, typeName) {
			return t.ID, nil
		}
	}

	return "", fmt.Errorf("type %q not found; available types: %s", typeName, strings.Join(typeNames, ", "))
}
