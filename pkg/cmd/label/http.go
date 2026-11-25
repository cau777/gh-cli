package label

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

const (
	maxPageSize = 100

	defaultOrder = "asc"
	defaultSort  = "created"
)

var defaultFields = []string{
	"color",
	"description",
	"name",
}

type listQueryOptions struct {
	Limit int
	Query string
	Order string
	Sort  string

	fields []string
}

type restLabel struct {
	NodeID      string `json:"node_id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Default     bool   `json:"default"`
}

// listLabels lists the labels in the given repo. Pass -1 for limit to list all labels;
// otherwise, only that number of labels is returned for any number of pages.
func listLabels(client *http.Client, repo ghrepo.Interface, opts listQueryOptions) ([]label, int, error) {
	if slices.Contains(opts.fields, "createdAt") || slices.Contains(opts.fields, "updatedAt") {
		return nil, 0, api.NotSupportedError("label createdAt and updatedAt fields are not supported with REST")
	}

	if opts.Order == "" {
		opts.Order = defaultOrder
	}
	if opts.Sort == "" {
		opts.Sort = defaultSort
	}

	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "labels")
	if err != nil {
		return nil, 0, err
	}
	u.SetQuery("per_page", strconv.Itoa(maxPageSize))

	apiClient := api.NewClientFromHTTP(client)
	var labels []label
	needle := strings.ToLower(opts.Query)

	nextPath := u.String()
	for nextPath != "" {
		var page []restLabel
		next, err := apiClient.RESTWithNext(repo.RepoHost(), http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, 0, err
		}

		for _, r := range page {
			if needle != "" &&
				!strings.Contains(strings.ToLower(r.Name), needle) &&
				!strings.Contains(strings.ToLower(r.Description), needle) {
				continue
			}
			labels = append(labels, label{
				ID:          r.NodeID,
				URL:         r.URL,
				Name:        r.Name,
				Description: r.Description,
				Color:       r.Color,
				IsDefault:   r.Default,
			})
		}

		nextPath = next
	}

	if opts.Sort == "name" {
		slices.SortStableFunc(labels, func(a, b label) int {
			cmp := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
			if opts.Order == "desc" {
				return -cmp
			}
			return cmp
		})
	} else if opts.Order == "desc" {
		slices.Reverse(labels)
	}

	totalCount := len(labels)
	if opts.Limit > 0 && len(labels) > opts.Limit {
		labels = labels[:opts.Limit]
	}

	return labels, totalCount, nil
}
