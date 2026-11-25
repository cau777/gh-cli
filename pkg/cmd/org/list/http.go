package list

import (
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

type OrganizationList struct {
	Organizations []Organization
	TotalCount    int
	User          string
}

type Organization struct {
	Login string
}

func listOrgs(httpClient *http.Client, hostname string, limit int) (*OrganizationList, error) {
	client := api.NewClientFromHTTP(httpClient)

	user, err := api.CurrentLoginName(client, hostname)
	if err != nil {
		return nil, err
	}

	u, err := safeurl.JoinPath("user", "orgs")
	if err != nil {
		return nil, err
	}
	perPage := min(limit, 100)
	if perPage < 1 {
		perPage = 30
	}
	u.SetQuery("per_page", strconv.Itoa(perPage))

	listResult := OrganizationList{
		User: user,
	}

	nextPath := u.String()
	for nextPath != "" {
		var page []Organization
		next, err := client.RESTWithNext(hostname, http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, err
		}

		for _, org := range page {
			listResult.Organizations = append(listResult.Organizations, org)
			if len(listResult.Organizations) == limit {
				listResult.TotalCount = len(listResult.Organizations)
				return &listResult, nil
			}
		}

		nextPath = next
	}

	listResult.TotalCount = len(listResult.Organizations)
	return &listResult, nil
}
