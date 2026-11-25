# Unsupported Features in REST Migration

This document tracks `gh` commands, subcommands, flags, `--json` fields, and
behavioral details that do not have a viable GitHub REST API replacement and
therefore either return `not all features are supported` or omit optional data
when running in REST-only mode.

## Pull Requests & Checks

- **`gh pr checks --required`**: GitHub REST check-runs (`/commits/{ref}/check-runs`) and commit status (`/commits/{ref}/status`) endpoints do not indicate which checks are required by branch protection rulesets. Passing `--required` returns `not all features are supported`.
- **PR Finder (`pkg/cmd/pr/shared/finder.go`)**: `projectItems`, `projectCards`, `closingIssuesReferences`, `isInMergeQueue`, and `isMergeQueueEnabled` are not available on REST PR endpoints and are left empty on default lookups. Explicit `--json` validation is handled at command entry points.
- **`gh pr list`, `gh pr view`, `gh pr status` `--json` fields**: Passing any of `projectItems`, `projectCards`, `closingIssuesReferences`, `reviewDecision`, or `potentialMergeCommit` with `--json` returns `not all features are supported`. Default (non-`--json`) views omit those fields gracefully.
- **`gh pr status` branch protection review/strictness indicators**: REST does not expose `reviewDecision` (`- Review required`) or `requiresStrictStatusChecks` (`✓ Up to date` / `- Not up to date`) / `requiredApprovingReviewCount` (`N/M Approved`) without admin branch-protection permissions. `Changes requested` and `N Approved` are derived from `LatestReviews` instead.
- **`gh pr ready` / `gh pr ready --undo`**: Converting between draft and ready-for-review state is a GraphQL-only mutation (`markPullRequestReadyForReview` / `convertPullRequestToDraft`) with no GitHub REST API endpoint. Both commands return `not all features are supported`.
- **`gh pr revert`**: Creating a revert pull request server-side is a GraphQL-only mutation (`revertPullRequest`) with no GitHub REST API endpoint. Returns `not all features are supported`.
- **`gh pr merge --auto` / `--disable-auto` / `--author-email` / merge queue**: Enabling/disabling auto-merge, setting a custom merge commit author email, and enqueuing pull requests into a merge queue have no GitHub REST API equivalents and return `not all features are supported`. Standard REST merges (`PUT /repos/{owner}/{repo}/pulls/{pull_number}/merge`) remain supported.
- **`gh pr update-branch --rebase`**: Updating a pull request branch via REST (`PUT /repos/{owner}/{repo}/pulls/{pull_number}/update-branch`) always performs a merge update and does not support rebasing. Passing `--rebase` returns `not all features are supported`.
- **`gh pr create --project` / `gh pr edit --add-project` / `gh pr edit --remove-project`**: Projects V2 associations and Projects V1 columns cannot be managed on pull requests via REST and return `not all features are supported`.
- **`@copilot` reviewer/assignee on pull requests**: Assigning or requesting Copilot (`copilot` / `@copilot`) on pull requests relies on GraphQL actor mutations (`replaceActorsForAssignable` / `requestReviews`) and returns `not all features are supported`.
- **Interactive reviewer/assignee metadata on `gh pr create` / `gh pr edit`**: Interactive reviewer and assignee prompts use static REST metadata (`GET /repos/{owner}/{repo}/assignees` and `GET /orgs/{owner}/teams`) rather than live GraphQL actor search (`SuggestedReviewerActors` / `SuggestedAssignableActors`).

## Issues

- **`gh issue list`, `gh issue view`, `gh issue status` `--json` fields**: Passing any of `projectCards`, `projectItems`, `closedByPullRequestsReferences`, or `isPinned` with `--json` returns `not all features are supported`. Default non-`--json` views omit those fields gracefully.
- **Comment moderation metadata (`gh issue view`)**: REST issue comment endpoints do not expose `isMinimized` or `minimizedReason`, so minimized-comment badges are not rendered.
- **`gh issue close --duplicate-of`**: Marking an issue as a duplicate of a specific issue (`duplicateIssueId`) is a GraphQL-only field on `closeIssue` and returns `not all features are supported`. Closing with `--reason duplicate` (`state_reason: "duplicate"`) remains supported via REST.
- **`gh issue delete`**: GitHub has no REST API endpoint to delete an issue. Returns `not all features are supported`.
- **`gh issue pin` / `gh issue unpin`**: Pinning and unpinning issues on a repository are GraphQL-only mutations (`pinIssue` / `unpinIssue`). Both return `not all features are supported`.
- **`gh issue transfer`**: Transferring an issue to another repository is a GraphQL-only mutation (`transferIssue`). Returns `not all features are supported`.
- **`gh issue develop`**: Creating and listing linked issue branches (`createLinkedBranch` / `linkedBranches`) are GraphQL-only APIs with no GitHub REST equivalent. Returns `not all features are supported`.
- **`gh issue create --project` / `gh issue edit --add-project` / `--remove-project`**: Projects V2 associations and Projects V1 columns cannot be managed on issues via REST and return `not all features are supported`.
- **`@copilot` assignee on issues**: Assigning or unassigning Copilot (`copilot` / `@copilot`) on issues relies on GraphQL actor mutations and returns `not all features are supported`.
- **Interactive assignee metadata on `gh issue create` / `gh issue edit`**: Interactive assignee prompts use static REST metadata (`GET /repos/{owner}/{repo}/assignees`) rather than live GraphQL actor search (`SearchRepoAssignableActors` / `SuggestedAssignableActors`).

## Repositories & Releases

- **`gh repo list` / `gh repo view` `--json` fields**: Passing any of `assignableUsers`, `codeOfConduct`, `contactLinks`, `fundingLinks`, `isBlankIssuesEnabled`, `isEmpty`, `isSecurityPolicyEnabled`, `isUserConfigurationRepository`, `issueTemplates`, `issues`, `labels`, `languages`, `latestRelease`, `mentionableUsers`, `milestones`, `projects`, `projectsV2`, `pullRequestTemplates`, `pullRequests`, `securityPolicyUrl`, `viewerDefaultCommitEmail`, `viewerDefaultMergeMethod`, `viewerHasStarred`, `viewerPossibleCommitEmails`, or `viewerSubscription` with `--json` returns `not all features are supported`.
- **`gh repo create --template --internal`**: GitHub's REST `POST /repos/{owner}/{repo}/generate` endpoint only accepts `private: bool` and cannot set `internal` visibility on template-generated repositories. Passing `--internal` with `--template` returns `not all features are supported`.
- **`gh repo read-dir` (`GET /repos/{owner}/{repo}/contents/{path}`)**: The REST Contents API caps directories at 1000 entries, omits executable permission bits (`100644` vs `100755`), omits directory `gitSHA`/`id` values, and omits submodule `branch` metadata.
- **`gh repo list`**: REST owner repository listing endpoints do not return a `totalCount` field; header totals reflect fetched repositories.
- **`gh release list --order asc`**: The REST `/repos/{owner}/{repo}/releases` endpoint returns releases in descending order without a sort-order parameter, so `--order asc` fetches all releases and sorts client-side by `created_at`.

## Projects & Discussions

*Tracked as Project and Discussion commands are migrated.*

## Other Commands & `gh api`

- **Feature detection (`internal/featuredetection`)**: GraphQL schema
  introspection (`__type`) was removed. Modern GitHub.com REST API
  (`2022-11-28`) capabilities are assumed; older GHES-specific GraphQL schema
  probes are no longer performed.
- **`gh label list` (`--json createdAt,updatedAt`, `--sort created`)**: GitHub's REST `/repos/{owner}/{repo}/labels` endpoint does not expose label `createdAt` or `updatedAt` timestamps. Requesting `createdAt` or `updatedAt` via `--json` returns `not all features are supported`; filtering (`--search`) and sorting (`--sort name` / `--order`) are performed client-side.
- **`gh org list`**: The REST `GET /user/orgs` endpoint does not return a `totalCount` field; header totals reflect fetched organizations.
- **`gh ruleset list`**: REST ruleset summary listings omit individual rule definitions when returned by `/rulesets`, so summary `RULES` counts are `0` when omitted, and `totalCount` reflects fetched rulesets.
- **`gh status`**: Assignment and review-request searches use REST `/search/issues`, which does not emit partial per-item GraphQL `FORBIDDEN` SAML warnings in 200 responses (HTTP 403 search responses still surface auth warnings).
- **`gh gist list --include-content`**: REST `GET /gists` summaries omit file contents; when `includeContent` is requested and file contents are absent in the list response, `gh` fetches individual gists via `GET /gists/{id}`.

## Shared Behavioral Caveats

- **`RepoFindForks` (`api/queries_repo.go`)**: GitHub's REST `GET /repos/{owner}/{repo}/forks` endpoint has no `affiliations=OWNER` filter, so `RepoFindForks` checks for a fork at the viewer's own login (`GET /repos/{viewer}/{repo}`) plus the first 100 forks sorted by newest instead of exhaustive owner-affiliated GraphQL pagination.
- **`RepoAssignableUsers` (`api/queries_repo.go`)**: GitHub's REST `GET /repos/{owner}/{repo}/assignees` endpoint returns only `login` and `node_id` (not the user's display `name`), so interactive assignee/reviewer prompts and completion entries display logins without display names.
