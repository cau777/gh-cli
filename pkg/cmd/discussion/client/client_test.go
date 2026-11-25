package client

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/stretchr/testify/require"
)

func TestDiscussionClientNotSupported(t *testing.T) {
	c := NewDiscussionClient(&http.Client{})
	repo := ghrepo.New("OWNER", "REPO")

	_, err := c.List(repo, ListFilters{}, "", 10)
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.Search(repo, SearchFilters{}, "", 10)
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.GetByNumber(repo, 1)
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.GetWithComments(repo, 1, 10, "", false)
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.GetCommentReplies("github.com", "DC_1", 10, "", false)
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.ListCategories(repo)
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.ListLabels(repo)
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.Create(repo, CreateDiscussionInput{Title: "t", Body: "b", CategoryID: "c"})
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.Update(repo, UpdateDiscussionInput{DiscussionID: "D_1"})
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.AddComment(repo, "D_1", "hi", "")
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.UpdateComment(repo, "DC_1", "hi")
	require.ErrorIs(t, err, api.ErrNotSupported)

	err = c.DeleteComment(repo, "DC_1")
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.GetComment("github.com", "DC_1")
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = c.ResolveCommentNodeID(repo, 123)
	require.ErrorIs(t, err, api.ErrNotSupported)
}
