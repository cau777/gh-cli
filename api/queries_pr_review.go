package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type PullRequestReviewState int

const (
	ReviewApprove PullRequestReviewState = iota
	ReviewRequestChanges
	ReviewComment
)

type PullRequestReviewInput struct {
	Body  string
	State PullRequestReviewState
}

type PullRequestReviews struct {
	Nodes    []PullRequestReview
	PageInfo struct {
		HasNextPage bool
		EndCursor   string
	}
	TotalCount int
}

type PullRequestReview struct {
	ID                  string         `json:"id"`
	Author              CommentAuthor  `json:"author"`
	AuthorAssociation   string         `json:"authorAssociation"`
	Body                string         `json:"body"`
	SubmittedAt         *time.Time     `json:"submittedAt"`
	IncludesCreatedEdit bool           `json:"includesCreatedEdit"`
	ReactionGroups      ReactionGroups `json:"reactionGroups"`
	State               string         `json:"state"`
	URL                 string         `json:"url,omitempty"`
	Commit              Commit         `json:"commit"`
}

func (prr PullRequestReview) Identifier() string {
	return prr.ID
}

func (prr PullRequestReview) AuthorLogin() string {
	return prr.Author.DisplayName()
}

func (prr PullRequestReview) Association() string {
	return prr.AuthorAssociation
}

func (prr PullRequestReview) Content() string {
	return prr.Body
}

func (prr PullRequestReview) Created() time.Time {
	if prr.SubmittedAt == nil {
		return time.Time{}
	}
	return *prr.SubmittedAt
}

func (prr PullRequestReview) HiddenReason() string {
	return ""
}

func (prr PullRequestReview) IsEdited() bool {
	return prr.IncludesCreatedEdit
}

func (prr PullRequestReview) IsHidden() bool {
	return false
}

func (prr PullRequestReview) Link() string {
	return prr.URL
}

func (prr PullRequestReview) Reactions() ReactionGroups {
	return prr.ReactionGroups
}

func (prr PullRequestReview) Status() string {
	return prr.State
}

type PullRequestReviewStatus struct {
	ChangesRequested bool
	Approved         bool
	ReviewRequired   bool
}

func (pr *PullRequest) ReviewStatus() PullRequestReviewStatus {
	var status PullRequestReviewStatus
	switch pr.ReviewDecision {
	case "CHANGES_REQUESTED":
		status.ChangesRequested = true
	case "APPROVED":
		status.Approved = true
	case "REVIEW_REQUIRED":
		status.ReviewRequired = true
	default:
		for _, review := range pr.LatestReviews.Nodes {
			switch review.State {
			case "CHANGES_REQUESTED":
				status.ChangesRequested = true
				status.Approved = false
				return status
			case "APPROVED":
				status.Approved = true
			}
		}
	}
	return status
}

func (pr *PullRequest) DisplayableReviews() PullRequestReviews {
	published := []PullRequestReview{}
	for _, prr := range pr.Reviews.Nodes {
		//Dont display pending reviews
		//Dont display commenting reviews without top level comment body
		if prr.State != "PENDING" && !(prr.State == "COMMENTED" && prr.Body == "") {
			published = append(published, prr)
		}
	}
	return PullRequestReviews{Nodes: published, TotalCount: len(published)}
}

type ReviewRequests struct {
	Nodes []struct {
		RequestedReviewer RequestedReviewer
	}
}

type RequestedReviewer struct {
	TypeName     string `json:"__typename"`
	Login        string `json:"login"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Organization struct {
		Login string `json:"login"`
	} `json:"organization"`
}

const teamTypeName = "Team"
const botTypeName = "Bot"
const userTypeName = "User"

func (r RequestedReviewer) LoginOrSlug() string {
	if r.TypeName == teamTypeName {
		return fmt.Sprintf("%s/%s", r.Organization.Login, r.Slug)
	}
	return r.Login
}

// DisplayName returns a user-friendly name for the reviewer via actorDisplayName.
// Teams are handled separately as "org/slug".
func (r RequestedReviewer) DisplayName() string {
	if r.TypeName == teamTypeName {
		return fmt.Sprintf("%s/%s", r.Organization.Login, r.Slug)
	}
	return actorDisplayName(r.TypeName, r.Login, r.Name)
}

func (r ReviewRequests) Logins() []string {
	logins := make([]string, len(r.Nodes))
	for i, r := range r.Nodes {
		logins[i] = r.RequestedReviewer.LoginOrSlug()
	}
	return logins
}

// DisplayNames returns user-friendly display names for all requested reviewers.
func (r ReviewRequests) DisplayNames() []string {
	names := make([]string, len(r.Nodes))
	for i, r := range r.Nodes {
		names[i] = r.RequestedReviewer.DisplayName()
	}
	return names
}

// ReviewerCandidate represents a potential reviewer for a pull request.
// This can be a User, Bot, or Team. It mirrors AssignableActor but adds
// team support (teams can review but not be assigned) and drops the ID method.
// ReviewerUser and ReviewerBot are thin wrappers around AssignableUser and
// AssignableBot that satisfy this interface.
type ReviewerCandidate interface {
	DisplayName() string
	Login() string

	sealedReviewerCandidate()
}

// ReviewerUser is a user who can review a pull request.
type ReviewerUser struct {
	AssignableUser
}

func NewReviewerUser(login, name string) ReviewerUser {
	return ReviewerUser{
		AssignableUser: NewAssignableUser("", login, name),
	}
}

func (r ReviewerUser) sealedReviewerCandidate() {}

// ReviewerBot is a bot who can review a pull request.
type ReviewerBot struct {
	AssignableBot
}

func NewReviewerBot(login string) ReviewerBot {
	return ReviewerBot{
		AssignableBot: NewAssignableBot("", login),
	}
}

func (b ReviewerBot) DisplayName() string {
	return actorDisplayName("Bot", b.login, "")
}

func (r ReviewerBot) sealedReviewerCandidate() {}

// ReviewerTeam is a team that can review a pull request.
type ReviewerTeam struct {
	org      string
	teamSlug string
}

// NewReviewerTeam creates a new ReviewerTeam.
func NewReviewerTeam(orgName, teamSlug string) ReviewerTeam {
	return ReviewerTeam{org: orgName, teamSlug: teamSlug}
}

func (r ReviewerTeam) DisplayName() string {
	return fmt.Sprintf("%s/%s", r.org, r.teamSlug)
}

func (r ReviewerTeam) Login() string {
	return fmt.Sprintf("%s/%s", r.org, r.teamSlug)
}

func (r ReviewerTeam) Slug() string {
	return r.teamSlug
}

func (r ReviewerTeam) sealedReviewerCandidate() {}

func AddReview(client *Client, repo ghrepo.Interface, pr *PullRequest, input *PullRequestReviewInput) error {
	event := "COMMENT"
	switch input.State {
	case ReviewApprove:
		event = "APPROVE"
	case ReviewRequestChanges:
		event = "REQUEST_CHANGES"
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "pulls", strconv.Itoa(pr.Number), "reviews")
	if err != nil {
		return err
	}

	body := struct {
		Body  string `json:"body"`
		Event string `json:"event"`
	}{
		Body:  input.Body,
		Event: event,
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}

	return client.REST(repo.RepoHost(), "POST", path.String(), buf, nil)
}

// AddPullRequestReviews adds the given user and team reviewers to a pull request using the REST API.
// Team identifiers can be in "org/slug" format.
func AddPullRequestReviews(client *Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}

	// The API requires empty arrays instead of null values
	if users == nil {
		users = []string{}
	}

	path, err := safeurl.JoinPath(
		"repos",
		repo.RepoOwner(),
		repo.RepoName(),
		"pulls",
		strconv.Itoa(prNumber),
		"requested_reviewers",
	)
	if err != nil {
		return err
	}
	body := struct {
		Reviewers     []string `json:"reviewers"`
		TeamReviewers []string `json:"team_reviewers"`
	}{
		Reviewers:     users,
		TeamReviewers: extractTeamSlugs(teams),
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}
	// The endpoint responds with the updated pull request object; we don't need it here.
	return client.REST(repo.RepoHost(), "POST", path.String(), buf, nil)
}

// RemovePullRequestReviews removes requested reviewers from a pull request using the REST API.
// Team identifiers can be in "org/slug" format.
func RemovePullRequestReviews(client *Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}

	// The API requires empty arrays instead of null values
	if users == nil {
		users = []string{}
	}

	path, err := safeurl.JoinPath(
		"repos",
		repo.RepoOwner(),
		repo.RepoName(),
		"pulls",
		strconv.Itoa(prNumber),
		"requested_reviewers",
	)
	if err != nil {
		return err
	}
	body := struct {
		Reviewers     []string `json:"reviewers"`
		TeamReviewers []string `json:"team_reviewers"`
	}{
		Reviewers:     users,
		TeamReviewers: extractTeamSlugs(teams),
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}
	// The endpoint responds with the updated pull request object; we don't need it here.
	return client.REST(repo.RepoHost(), "DELETE", path.String(), buf, nil)
}

// extractTeamSlugs extracts just the slug portion from team identifiers.
// Team identifiers can be in "org/slug" format; this returns just the slug.
func extractTeamSlugs(teams []string) []string {
	slugs := make([]string, 0, len(teams))
	for _, t := range teams {
		if t == "" {
			continue
		}
		s := strings.SplitN(t, "/", 2)
		slugs = append(slugs, s[len(s)-1])
	}
	return slugs
}
