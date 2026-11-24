# Unsupported Features in REST Migration

This document tracks `gh` commands, subcommands, flags, `--json` fields, and
behavioral details that do not have a viable GitHub REST API replacement and
therefore either return `not all features are supported` or omit optional data
when running in REST-only mode.

## Pull Requests & Checks

*Tracked as PR commands are migrated.*

## Issues

*Tracked as Issue commands are migrated.*

## Repositories & Releases

*Tracked as Repository and Release commands are migrated.*

## Projects & Discussions

*Tracked as Project and Discussion commands are migrated.*

## Other Commands & `gh api`

- **Feature detection (`internal/featuredetection`)**: GraphQL schema
  introspection (`__type`) was removed. Modern GitHub.com REST API
  (`2022-11-28`) capabilities are assumed; older GHES-specific GraphQL schema
  probes are no longer performed.

## Shared Behavioral Caveats

- **`RepoFindForks` (`api/queries_repo.go`)**: GitHub's REST `GET /repos/{owner}/{repo}/forks` endpoint has no `affiliations=OWNER` filter, so `RepoFindForks` checks for a fork at the viewer's own login (`GET /repos/{viewer}/{repo}`) plus the first 100 forks sorted by newest instead of exhaustive owner-affiliated GraphQL pagination.
- **`RepoAssignableUsers` (`api/queries_repo.go`)**: GitHub's REST `GET /repos/{owner}/{repo}/assignees` endpoint returns only `login` and `node_id` (not the user's display `name`), so interactive assignee/reviewer prompts and completion entries display logins without display names.
