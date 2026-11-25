// Package client provides an abstraction layer for interacting with GitHub Discussions.
// GitHub Discussions has no REST API equivalent, so requests return api.ErrNotSupported.
package client

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

//go:generate moq -rm -out client_mock.go . DiscussionClient

// DiscussionClient defines operations for interacting with the GitHub Discussions API.
type DiscussionClient interface {
	// List returns discussions in a repository matching the given filters.
	List(repo ghrepo.Interface, filters ListFilters, after string, limit int) (*DiscussionListResult, error)
	// Search returns discussions in a repository matching the given search filters.
	Search(repo ghrepo.Interface, filters SearchFilters, after string, limit int) (*DiscussionListResult, error)
	// GetByNumber returns a single discussion by its number.
	GetByNumber(repo ghrepo.Interface, number int32) (*Discussion, error)
	// GetWithComments returns a discussion along with a page of its comments.
	GetWithComments(repo ghrepo.Interface, number int32, commentLimit int, after string, newest bool) (*Discussion, error)
	// GetCommentReplies returns a comment's parent discussion along with a page of the comment's replies.
	GetCommentReplies(host string, commentID string, limit int, after string, newest bool) (*Discussion, error)
	// ListCategories returns the discussion categories available in a repository.
	ListCategories(repo ghrepo.Interface) ([]DiscussionCategory, error)
	// ListLabels returns the labels available in a repository.
	ListLabels(repo ghrepo.Interface) ([]DiscussionLabel, error)
	// Create creates a discussion.
	Create(repo ghrepo.Interface, input CreateDiscussionInput) (*Discussion, error)
	// Update updates a discussion.
	Update(repo ghrepo.Interface, input UpdateDiscussionInput) (*Discussion, error)
	// AddComment adds a comment or reply to a discussion.
	AddComment(repo ghrepo.Interface, discussionID, body, replyToID string) (*DiscussionComment, error)
	// UpdateComment updates the body of an existing discussion comment or reply.
	UpdateComment(repo ghrepo.Interface, commentID, body string) (*DiscussionComment, error)
	// DeleteComment deletes a discussion comment or reply.
	DeleteComment(repo ghrepo.Interface, commentID string) error
	// GetComment fetches a single discussion comment by node ID.
	GetComment(host string, commentID string) (*DiscussionComment, error)
	// ResolveCommentNodeID constructs a discussion comment node ID from a
	// repository and a comment database ID.
	ResolveCommentNodeID(repo ghrepo.Interface, commentDatabaseID int64) (string, error)
}

type discussionClient struct{}

// NewDiscussionClient creates a DiscussionClient.
func NewDiscussionClient(_ *http.Client) DiscussionClient {
	return &discussionClient{}
}

func errDiscussionsNotSupported() error {
	return api.NotSupportedError("GitHub Discussions is not supported with REST")
}

func (c *discussionClient) List(_ ghrepo.Interface, _ ListFilters, _ string, _ int) (*DiscussionListResult, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) Search(_ ghrepo.Interface, _ SearchFilters, _ string, _ int) (*DiscussionListResult, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) GetByNumber(_ ghrepo.Interface, _ int32) (*Discussion, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) GetWithComments(_ ghrepo.Interface, _ int32, _ int, _ string, _ bool) (*Discussion, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) GetCommentReplies(_ string, _ string, _ int, _ string, _ bool) (*Discussion, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) ListCategories(_ ghrepo.Interface) ([]DiscussionCategory, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) ListLabels(_ ghrepo.Interface) ([]DiscussionLabel, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) Create(_ ghrepo.Interface, _ CreateDiscussionInput) (*Discussion, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) Update(_ ghrepo.Interface, _ UpdateDiscussionInput) (*Discussion, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) AddComment(_ ghrepo.Interface, _, _, _ string) (*DiscussionComment, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) UpdateComment(_ ghrepo.Interface, _, _ string) (*DiscussionComment, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) DeleteComment(_ ghrepo.Interface, _ string) error {
	return errDiscussionsNotSupported()
}

func (c *discussionClient) GetComment(_ string, _ string) (*DiscussionComment, error) {
	return nil, errDiscussionsNotSupported()
}

func (c *discussionClient) ResolveCommentNodeID(_ ghrepo.Interface, _ int64) (string, error) {
	return "", errDiscussionsNotSupported()
}
