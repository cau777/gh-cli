package list

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

var releaseFields = []string{
	"name",
	"tagName",
	"isDraft",
	"isLatest",
	"isPrerelease",
	"isImmutable",
	"createdAt",
	"publishedAt",
}

type Release struct {
	Name         string
	TagName      string
	IsDraft      bool
	IsImmutable  bool
	IsLatest     bool
	IsPrerelease bool
	CreatedAt    time.Time
	PublishedAt  time.Time
}

func (r *Release) ExportData(fields []string) map[string]any {
	return cmdutil.StructExportData(r, fields)
}

type restRelease struct {
	Name         string    `json:"name"`
	TagName      string    `json:"tag_name"`
	IsDraft      bool      `json:"draft"`
	IsImmutable  bool      `json:"immutable"`
	IsPrerelease bool      `json:"prerelease"`
	CreatedAt    time.Time `json:"created_at"`
	PublishedAt  time.Time `json:"published_at"`
}

func fetchReleases(httpClient *http.Client, repo ghrepo.Interface, limit int, excludeDrafts bool, excludePreReleases bool, order string, _ fd.ReleaseFeatures) ([]Release, error) {
	client := api.NewClientFromHTTP(httpClient)

	latestTag := ""
	latestPath, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", "latest")
	if err != nil {
		return nil, err
	}
	var latest restRelease
	if err := client.REST(repo.RepoHost(), http.MethodGet, latestPath.String(), nil, &latest); err == nil {
		latestTag = latest.TagName
	} else {
		var httpErr api.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
			return nil, err
		}
	}

	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases")
	if err != nil {
		return nil, err
	}
	perPage := 100
	ascOrder := strings.EqualFold(order, "asc")
	if !ascOrder && !excludeDrafts && !excludePreReleases && limit > 0 && limit < 100 {
		perPage = limit
	}
	u.SetQuery("per_page", strconv.Itoa(perPage))

	var releases []Release
	nextPath := u.String()
	for nextPath != "" {
		var page []restRelease
		next, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, nextPath, nil, &page)
		if err != nil {
			return nil, err
		}

		for _, r := range page {
			if excludeDrafts && r.IsDraft {
				continue
			}
			if excludePreReleases && r.IsPrerelease {
				continue
			}
			releases = append(releases, Release{
				Name:         r.Name,
				TagName:      r.TagName,
				IsDraft:      r.IsDraft,
				IsImmutable:  r.IsImmutable,
				IsLatest:     latestTag != "" && !r.IsDraft && !r.IsPrerelease && r.TagName == latestTag,
				IsPrerelease: r.IsPrerelease,
				CreatedAt:    r.CreatedAt,
				PublishedAt:  r.PublishedAt,
			})
			if !ascOrder && len(releases) == limit {
				return releases, nil
			}
		}

		nextPath = next
	}

	if ascOrder {
		slices.SortStableFunc(releases, func(a, b Release) int {
			return a.CreatedAt.Compare(b.CreatedAt)
		})
		if limit > 0 && len(releases) > limit {
			releases = releases[:limit]
		}
	}

	return releases, nil
}
