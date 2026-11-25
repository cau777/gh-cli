package list

import (
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/search"
)

type RepositoryList struct {
	Owner        string
	Repositories []api.Repository
	TotalCount   int
	FromSearch   bool
}

type FilterOptions struct {
	Visibility  string // private, public, internal
	Fork        bool
	Source      bool
	Language    string
	Topic       []string
	Archived    bool
	NonArchived bool
	Fields      []string
}

func listRepos(client *http.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	if filter.Language != "" || filter.Archived || filter.NonArchived || len(filter.Topic) > 0 || filter.Visibility == "internal" {
		return searchRepos(client, hostname, limit, owner, filter)
	}

	apiClient := api.NewClientFromHTTP(client)
	ownerLogin, repos, err := api.ListOwnerRepositoriesREST(apiClient, hostname, owner, limit, filter.Visibility, filter.Fork, filter.Source)
	if err != nil {
		return nil, err
	}

	return &RepositoryList{
		Owner:        ownerLogin,
		Repositories: repos,
		TotalCount:   len(repos),
	}, nil
}

func searchRepos(client *http.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	apiClient := api.NewClientFromHTTP(client)
	totalCount, repos, err := api.SearchRepositoriesREST(apiClient, hostname, searchQuery(owner, filter), limit)
	if err != nil {
		return nil, err
	}

	listResult := RepositoryList{
		Repositories: repos,
		TotalCount:   totalCount,
		FromSearch:   true,
	}
	for _, repo := range repos {
		if repo.NameWithOwner != "" {
			if idx := strings.IndexRune(repo.NameWithOwner, '/'); idx >= 0 {
				listResult.Owner = repo.NameWithOwner[:idx]
				break
			}
		}
	}

	return &listResult, nil
}

func searchQuery(owner string, filter FilterOptions) string {
	if owner == "" {
		owner = "@me"
	}

	fork := "true"
	if filter.Fork {
		fork = "only"
	} else if filter.Source {
		fork = "false"
	}

	var archived *bool
	if filter.Archived {
		trueBool := true
		archived = &trueBool
	}
	if filter.NonArchived {
		falseBool := false
		archived = &falseBool
	}

	q := search.Query{
		Keywords: []string{"sort:updated-desc"},
		Qualifiers: search.Qualifiers{
			Archived: archived,
			Fork:     fork,
			Is:       []string{filter.Visibility},
			Language: filter.Language,
			Topic:    filter.Topic,
			User:     []string{owner},
		},
	}

	return q.StandardSearchString()
}
