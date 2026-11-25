package api

import (
	"github.com/cli/cli/v2/internal/ghrepo"
)

type LinkedBranch struct {
	BranchName string
	URL        string
}

func CreateLinkedBranch(client *Client, host string, repoID, issueID, branchID, branchName string) (string, error) {
	return "", NotSupportedError("linked issue branches (gh issue develop) are not supported with REST")
}

func ListLinkedBranches(client *Client, repo ghrepo.Interface, issueNumber int) ([]LinkedBranch, error) {
	return nil, NotSupportedError("linked issue branches (gh issue develop) are not supported with REST")
}

func CheckLinkedBranchFeature(client *Client, host string) error {
	return NotSupportedError("linked issue branches (gh issue develop) are not supported with REST")
}

func FindRepoBranchID(client *Client, repo ghrepo.Interface, ref string) (string, string, error) {
	return "", "", NotSupportedError("linked issue branches (gh issue develop) are not supported with REST")
}
