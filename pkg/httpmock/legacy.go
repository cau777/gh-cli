package httpmock

import (
	"fmt"
)

// TODO: clean up methods in this file when there are no more callers

func restPermissionsJSON(permission string) string {
	switch permission {
	case "ADMIN":
		return `{"admin":true,"maintain":true,"push":true,"triage":true,"pull":true}`
	case "MAINTAIN":
		return `{"maintain":true,"push":true,"triage":true,"pull":true}`
	case "WRITE":
		return `{"push":true,"triage":true,"pull":true}`
	case "TRIAGE":
		return `{"triage":true,"pull":true}`
	case "READ":
		return `{"pull":true}`
	default:
		return `{}`
	}
}

func restRepoJSON(owner, repo, branch, permission string) string {
	return fmt.Sprintf(`{
		"node_id": "REPOID",
		"id": 1234,
		"name": "%s",
		"full_name": "%s/%s",
		"owner": {"login": "%s"},
		"description": "",
		"default_branch": "%s",
		"has_issues": true,
		"permissions": %s
	}`, repo, owner, repo, owner, branch, restPermissionsJSON(permission))
}

func (r *Registry) StubRepoInfoResponse(owner, repo, branch string) {
	r.StubRepoInfoResponseWithPermission(owner, repo, branch, "WRITE")
}

// StubRepoInfoResponseWithPermission is StubRepoInfoResponse with a
// caller-selected viewer permission.
func (r *Registry) StubRepoInfoResponseWithPermission(owner, repo, branch, permission string) {
	r.Register(
		REST("GET", fmt.Sprintf("repos/%s/%s", owner, repo)),
		StringResponse(restRepoJSON(owner, repo, branch, permission)))
}

func (r *Registry) StubIssueRepoInfoResponse(owner, repo string) {
	r.Register(
		REST("GET", fmt.Sprintf("repos/%s/%s", owner, repo)),
		StringResponse(restRepoJSON(owner, repo, "main", "WRITE")))
}

func (r *Registry) StubRepoResponse(owner, repo string) {
	r.StubRepoResponseWithPermission(owner, repo, "WRITE")
}

func (r *Registry) StubRepoResponseWithPermission(owner, repo, permission string) {
	r.Register(REST("GET", "user"), StringResponse(`{"login":"OWNER"}`))
	r.Register(
		REST("GET", fmt.Sprintf("repos/%s/%s", owner, repo)),
		StringResponse(restRepoJSON(owner, repo, "master", permission)))
}
