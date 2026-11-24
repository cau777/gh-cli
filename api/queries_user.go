package api

import (
	"net/http"

	"github.com/cli/cli/v2/internal/safeurl"
)

type Organization struct {
	Login string
}

// CurrentLoginName returns the login of the authenticated user via REST.
func CurrentLoginName(client *Client, hostname string) (string, error) {
	path, err := safeurl.JoinPath("user")
	if err != nil {
		return "", err
	}
	var user restUser
	if err := client.REST(hostname, http.MethodGet, path.String(), nil, &user); err != nil {
		return "", err
	}
	return user.Login, nil
}

// CurrentLoginNameAndOrgs returns the authenticated user's login and organization logins via REST.
func CurrentLoginNameAndOrgs(client *Client, hostname string) (string, []string, error) {
	login, err := CurrentLoginName(client, hostname)
	if err != nil {
		return "", nil, err
	}

	orgsPath, err := safeurl.JoinPath("user", "orgs")
	if err != nil {
		return "", nil, err
	}
	orgsPath.SetQuery("per_page", "100")

	orgs, err := restPaginate[struct {
		Login string `json:"login"`
	}](client, hostname, orgsPath.String(), 0)
	if err != nil {
		return "", nil, err
	}

	orgNames := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgNames = append(orgNames, org.Login)
	}
	return login, orgNames, nil
}

// CurrentUserID returns the node ID of the authenticated user via REST.
func CurrentUserID(client *Client, hostname string) (string, error) {
	path, err := safeurl.JoinPath("user")
	if err != nil {
		return "", err
	}
	var user restUser
	if err := client.REST(hostname, http.MethodGet, path.String(), nil, &user); err != nil {
		return "", err
	}
	return user.NodeID, nil
}
