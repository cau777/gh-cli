package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type Comments struct {
	Nodes      []Comment
	TotalCount int
	PageInfo   struct {
		HasNextPage bool
		EndCursor   string
	}
}

func (cs Comments) CurrentUserComments() []Comment {
	var comments []Comment
	for _, c := range cs.Nodes {
		if c.ViewerDidAuthor {
			comments = append(comments, c)
		}
	}
	return comments
}

type Comment struct {
	ID                  string         `json:"id"`
	DatabaseID          int64          `json:"databaseId,omitempty"`
	Author              CommentAuthor  `json:"author"`
	AuthorAssociation   string         `json:"authorAssociation"`
	Body                string         `json:"body"`
	CreatedAt           time.Time      `json:"createdAt"`
	IncludesCreatedEdit bool           `json:"includesCreatedEdit"`
	IsMinimized         bool           `json:"isMinimized"`
	MinimizedReason     string         `json:"minimizedReason"`
	ReactionGroups      ReactionGroups `json:"reactionGroups"`
	URL                 string         `json:"url,omitempty"`
	ViewerDidAuthor     bool           `json:"viewerDidAuthor"`
}

type CommentCreateInput struct {
	Body      string
	SubjectId string
	Repo      ghrepo.Interface
	Number    int
}

type CommentDeleteInput struct {
	CommentId         string
	Repo              ghrepo.Interface
	CommentDatabaseID int64
	CommentURL        string
}

type CommentUpdateInput struct {
	Body              string
	CommentId         string
	Repo              ghrepo.Interface
	CommentDatabaseID int64
	CommentURL        string
}

var issueCommentURLRE = regexp.MustCompile(`#issuecomment-(\d+)$`)

func resolveRESTCommentID(databaseID int64, commentID, commentURL string) (string, error) {
	if databaseID > 0 {
		return strconv.FormatInt(databaseID, 10), nil
	}
	if m := issueCommentURLRE.FindStringSubmatch(commentURL); len(m) == 2 {
		return m[1], nil
	}
	if _, err := strconv.ParseInt(commentID, 10, 64); err == nil {
		return commentID, nil
	}
	return "", errors.New("cannot determine numeric REST comment ID")
}

func CommentCreate(client *Client, repoHost string, params CommentCreateInput) (string, error) {
	if params.Repo == nil || params.Number <= 0 {
		return "", errors.New("cannot create comment via REST without repository and issue/pull request number")
	}

	path, err := safeurl.JoinPath("repos", params.Repo.RepoOwner(), params.Repo.RepoName(), "issues", strconv.Itoa(params.Number), "comments")
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: params.Body})
	if err != nil {
		return "", err
	}

	var resp struct {
		HTMLURL string `json:"html_url"`
	}
	host := repoHost
	if host == "" {
		host = params.Repo.RepoHost()
	}
	if err := client.REST(host, http.MethodPost, path.String(), bytes.NewReader(payload), &resp); err != nil {
		return "", err
	}
	return resp.HTMLURL, nil
}

func CommentUpdate(client *Client, repoHost string, params CommentUpdateInput) (string, error) {
	if params.Repo == nil {
		return "", errors.New("cannot update comment via REST without repository")
	}
	id, err := resolveRESTCommentID(params.CommentDatabaseID, params.CommentId, params.CommentURL)
	if err != nil {
		return "", err
	}

	path, err := safeurl.JoinPath("repos", params.Repo.RepoOwner(), params.Repo.RepoName(), "issues", "comments", id)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: params.Body})
	if err != nil {
		return "", err
	}

	var resp struct {
		HTMLURL string `json:"html_url"`
	}
	host := repoHost
	if host == "" {
		host = params.Repo.RepoHost()
	}
	if err := client.REST(host, http.MethodPatch, path.String(), bytes.NewReader(payload), &resp); err != nil {
		return "", err
	}
	return resp.HTMLURL, nil
}

func CommentDelete(client *Client, repoHost string, params CommentDeleteInput) error {
	if params.Repo == nil {
		return errors.New("cannot delete comment via REST without repository")
	}
	id, err := resolveRESTCommentID(params.CommentDatabaseID, params.CommentId, params.CommentURL)
	if err != nil {
		return err
	}

	path, err := safeurl.JoinPath("repos", params.Repo.RepoOwner(), params.Repo.RepoName(), "issues", "comments", id)
	if err != nil {
		return err
	}

	host := repoHost
	if host == "" {
		host = params.Repo.RepoHost()
	}
	return client.REST(host, http.MethodDelete, path.String(), nil, nil)
}

func (c Comment) Identifier() string {
	return c.ID
}

func (c Comment) AuthorLogin() string {
	return c.Author.DisplayName()
}

func (c Comment) Association() string {
	return c.AuthorAssociation
}

func (c Comment) Content() string {
	return c.Body
}

func (c Comment) Created() time.Time {
	return c.CreatedAt
}

func (c Comment) HiddenReason() string {
	return c.MinimizedReason
}

func (c Comment) IsEdited() bool {
	return c.IncludesCreatedEdit
}

func (c Comment) IsHidden() bool {
	return c.IsMinimized
}

func (c Comment) Link() string {
	return c.URL
}

func (c Comment) Reactions() ReactionGroups {
	return c.ReactionGroups
}

func (c Comment) Status() string {
	return ""
}
