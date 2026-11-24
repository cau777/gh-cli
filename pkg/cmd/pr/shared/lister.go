package shared

import (
	"fmt"
	"net/http"

	api "github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type PRLister interface {
	List(opt ListOptions) (*api.PullRequestAndTotalCount, error)
}

type ListOptions struct {
	BaseRepo ghrepo.Interface

	LimitResults int

	State      string
	BaseBranch string
	HeadBranch string

	Fields []string
}

type lister struct {
	httpClient *http.Client
}

func NewLister(httpClient *http.Client) PRLister {
	return &lister{
		httpClient: httpClient,
	}
}

func (l *lister) List(opts ListOptions) (*api.PullRequestAndTotalCount, error) {
	client := api.NewClientFromHTTP(l.httpClient)
	return api.ListPullRequestsREST(client, opts.BaseRepo, api.ListPullRequestsOptions{
		State:      opts.State,
		BaseBranch: opts.BaseBranch,
		HeadBranch: opts.HeadBranch,
		Limit:      opts.LimitResults,
		Fields:     opts.Fields,
	})
}

type mockLister struct {
	called       bool
	expectFields []string

	result *api.PullRequestAndTotalCount
	err    error
}

func NewMockLister(result *api.PullRequestAndTotalCount, err error) *mockLister {
	return &mockLister{
		result: result,
		err:    err,
	}
}

func (m *mockLister) List(opt ListOptions) (*api.PullRequestAndTotalCount, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}

	if m.expectFields != nil {

		if !isEqualSet(m.expectFields, opt.Fields) {
			return nil, fmt.Errorf("unexpected fields: %v", opt.Fields)
		}
	}

	return m.result, m.err
}

func (m *mockLister) ExpectFields(fields []string) {
	m.expectFields = fields
}
