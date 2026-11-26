# Behavioral Differences from Upstream `gh`

This branch (`migrate-to-rest`) modifies upstream `gh` behavior to make API
requests compatible with REST-only environments, credential-injection proxies,
and URL-policy proxies.

For feature-level differences resulting from the removal of GraphQL, see
[UNSUPPORTED_FEATURES.md](UNSUPPORTED_FEATURES.md).

## 1. Unauthenticated Execution Without Login State or `GH_TOKEN`

Upstream `gh` refuses to run most commands unless at least one authenticated host
is configured or a token environment variable (such as `GH_TOKEN` or
`GITHUB_TOKEN`) is set, failing early with an authentication prompt.

In this branch:

- **No pre-run auth gate (`pkg/cmd/root/root.go`)**: Commands execute even when
  there is no local login state and no `GH_TOKEN` / `GITHUB_TOKEN` configured.
- **No `Authorization` header sent (`api/http_client.go`)**: When no token is
  resolved for the target host (or after a cross-host redirect), `gh` omits the
  `Authorization` header completely instead of sending an empty header or dummy
  token. This allows an external credential-injection proxy to attach
  credentials upstream.
- **Default host fallback for git remotes (`pkg/cmd/factory/remote_resolver.go`)**:
  Repository resolution from git remotes still resolves against `github.com` (or
  `GH_HOST` if set) when zero authenticated hosts are configured.

## 2. Consistent Named Resource Paths Across REST Pagination

GitHub's REST API returns `Link` headers whose `rel="next"` URLs often
canonicalize named resource paths into numeric IDs, for example:

- First request: `/repos/cau777/gh-rest/actions/workflows?per_page=1`
- Server `Link` header: `/repositories/1345296475/actions/workflows?per_page=1&page=2`

URL-scoped proxies that allowlist `/repos/<owner>/<repo>/...` reject subsequent
page requests sent to `/repositories/<id>/...`.

In this branch:

- **Path rewriting on `rel="next"` (`api/client.go`, `pkg/cmd/api/pagination.go`,
  `pkg/cmd/release/shared/fetch.go`, `internal/codespaces/api/api.go`)**: When
  extracting the next-page URL from a `Link` header, `RewriteNextPageURL`
  detects GitHub canonical ID paths (`/repositories/<id>/...`,
  `/organizations/<id>/...`, `/user/<id>/...`) that share the same API prefix
  (including GHES `/api/v3`) and trailing sub-path as the originating request,
  and rewrites the path back to the originating named path (`/repos/<owner>/<repo>/...`,
  `/orgs/<org>/...`, `/users/<user>/...`) while preserving all query parameters
  from the `Link` header (such as `page`, `per_page`, or cursors).
