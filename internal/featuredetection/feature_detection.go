package featuredetection

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/hashicorp/go-version"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

type Detector interface {
	IssueFeatures() (IssueFeatures, error)
	PullRequestFeatures() (PullRequestFeatures, error)
	RepositoryFeatures() (RepositoryFeatures, error)
	ProjectsV1() gh.ProjectsV1Support
	ProjectFeatures() (ProjectFeatures, error)
	SearchFeatures() (SearchFeatures, error)
	ReleaseFeatures() (ReleaseFeatures, error)
	ActionsFeatures() (ActionsFeatures, error)
	AuthFeatures() (AuthFeatures, error)
}

type IssueFeatures struct {
	// TODO ApiActorsSupported
	// ApiActorsSupported indicates the host supports actor-based APIs. True for
	// github.com and ghe.com, false for GHES.
	//
	// The GitHub API has two generations of assignee/reviewer types:
	//
	// Legacy (GHES): Uses AssignableUser (users only) and node-ID-based mutations.
	//   - assignableUsers query returns []AssignableUser
	//   - Mutations take node IDs (assigneeIds, userReviewerIds, teamReviewerIds)
	//
	// Actor-based (github.com): Uses AssignableActor (User + Bot union) and
	// login-based mutations, enabling assignment of non-user actors like Copilot.
	//   - suggestedActors query returns []AssignableActor (User | Bot)
	//   - suggestedReviewerActors returns []ReviewerCandidate (User | Bot | Team)
	//   - Mutations take logins (replaceActorsForAssignable, requestReviewsByLogin)
	//
	// When GHES adds support for the actor-based types and mutations, this flag
	// can be removed and all // TODO ApiActorsSupported sites collapsed to the
	// actor-only path. To verify GHES support, check whether the GHES GraphQL
	// schema includes:
	//   - The suggestedActors field on Repository (assignee search)
	//   - The suggestedReviewerActors field on PullRequest (reviewer search)
	//   - The replaceActorsForAssignable mutation
	//   - The requestReviewsByLogin mutation
	ApiActorsSupported bool

	// TODO IssueRelationshipsCleanup - remove when GHES 3.18 support ends (~October 2026)
	// IssueRelationshipsSupported indicates the host supports issue
	// relationships (blocked-by/blocking). Available on github.com and
	// GHES 3.19+. Issue types and sub-issues are GA on all supported GHES
	// versions (3.17+) and do not need feature detection.
	IssueRelationshipsSupported bool
}

var allIssueFeatures = IssueFeatures{
	ApiActorsSupported:          true,
	IssueRelationshipsSupported: true,
}

type PullRequestFeatures struct {
	MergeQueue bool
	// CheckRunAndStatusContextCounts indicates whether the API supports
	// the checkRunCount, checkRunCountsByState, statusContextCount and statusContextCountsByState
	// fields on the StatusCheckRollupContextConnection
	CheckRunAndStatusContextCounts bool
	CheckRunEvent                  bool
}

var allPullRequestFeatures = PullRequestFeatures{
	MergeQueue:                     true,
	CheckRunAndStatusContextCounts: true,
	CheckRunEvent:                  true,
}

type RepositoryFeatures struct {
	PullRequestTemplateQuery bool
	VisibilityField          bool
	AutoMerge                bool
}

var allRepositoryFeatures = RepositoryFeatures{
	PullRequestTemplateQuery: true,
	VisibilityField:          true,
	AutoMerge:                true,
}

type ProjectFeatures struct {
	// ProjectItemQuery indicates support for the `query` argument on
	// ProjectV2.items (supported on github.com and GHES 3.20+).
	ProjectItemQuery bool
}

var allProjectFeatures = ProjectFeatures{
	ProjectItemQuery: true,
}

type SearchFeatures struct {
	// AdvancedIssueSearch indicates whether the host supports advanced issue
	// search via API calls.
	AdvancedIssueSearchAPI bool
	// AdvancedIssueSearchOptIn indicates whether the host supports advanced
	// issue search as an opt-in feature, which has to be explicitly enabled in
	// API calls.
	AdvancedIssueSearchAPIOptIn bool

	// SemanticSearch indicates whether the host supports semantic issue search
	// (search_type=semantic). Dotcom-only; absent on single-tenant GHES.
	SemanticSearch bool
	// HybridSearch indicates whether the host supports hybrid issue search
	// (search_type=hybrid). Dotcom-only; absent on single-tenant GHES.
	HybridSearch bool

	// TODO advancedSearchFuture
	// When advanced issue search is supported in Pull Requests tab, or in
	// global search we can introduce more fields to reflect the support status.
}

// advancedIssueSearchNotSupported mimics GHE <3.18 where advanced issue search
// is either not supported or is not meant to be used due to not being stable
// enough (i.e. in preview).
var advancedIssueSearchNotSupported = SearchFeatures{
	AdvancedIssueSearchAPI: false,
}

// advancedIssueSearchSupportedAsOptIn mimics github.com and GHE >=3.18 before
// the full cleanup of temp types (i.e. ISSUE_ADVANCED search type is still
// present on the schema).
var advancedIssueSearchSupportedAsOptIn = SearchFeatures{
	AdvancedIssueSearchAPI:      true,
	AdvancedIssueSearchAPIOptIn: true,
}

// advancedIssueSearchSupportedAsOnlyBackend mimics github.com and GHE >=3.18
// after the full cleanup of temp types (i.e. ISSUE_ADVANCED search type is
// removed from the schema).
var advancedIssueSearchSupportedAsOnlyBackend = SearchFeatures{
	AdvancedIssueSearchAPI:      true,
	AdvancedIssueSearchAPIOptIn: false,
}

// semanticSearchSupported mimics a Dotcom host (github.com or ghe.com data
// residency) where semantic and hybrid issue search are available.
var semanticSearchSupported = SearchFeatures{
	AdvancedIssueSearchAPI: true,
	SemanticSearch:         true,
	HybridSearch:           true,
}

// semanticSearchUnsupported mimics a single-tenant GHES host where advanced
// issue search is available but semantic and hybrid search are not.
var semanticSearchUnsupported = SearchFeatures{
	AdvancedIssueSearchAPI: true,
	SemanticSearch:         false,
	HybridSearch:           false,
}

type ReleaseFeatures struct {
	ImmutableReleases bool
}

type ActionsFeatures struct {
	// DispatchRunDetails indicates whether the API supports the `return_run_details`
	// field in workflow dispatches that, when set to true, will return the details
	// of the created workflow run in the response (with status code 200).
	//
	// On older API versions (e.g. GHES 3.20 or earlier), this new field is not
	// supported and setting it will cause an error.
	DispatchRunDetails bool
}

// AuthFeatures describes the host's OAuth capabilities.
type AuthFeatures struct {
	// TODO expiringTokensCleanup - remove once GHES 3.22 support ends.
	// ExpiringTokens indicates support for offline_access, available on GHES 3.23+.
	ExpiringTokens bool
}

type detector struct {
	host       string
	httpClient *http.Client
}

func NewDetector(httpClient *http.Client, host string) Detector {
	return &detector{
		httpClient: httpClient,
		host:       host,
	}
}

func (d *detector) IssueFeatures() (IssueFeatures, error) {
	return IssueFeatures{
		ApiActorsSupported:          false,
		IssueRelationshipsSupported: true,
	}, nil
}

func (d *detector) PullRequestFeatures() (PullRequestFeatures, error) {
	return PullRequestFeatures{
		MergeQueue:                     false,
		CheckRunAndStatusContextCounts: true,
		CheckRunEvent:                  true,
	}, nil
}

func (d *detector) RepositoryFeatures() (RepositoryFeatures, error) {
	return allRepositoryFeatures, nil
}

const (
	enterpriseProjectsV1Removed = "3.17.0"
)

func (d *detector) ProjectsV1() gh.ProjectsV1Support {
	if !ghauth.IsEnterprise(d.host) {
		return gh.ProjectsV1Unsupported
	}

	hostVersion, hostVersionErr := resolveEnterpriseVersion(d.httpClient, d.host)
	v1ProjectCutoffVersion, v1ProjectCutoffVersionErr := version.NewVersion(enterpriseProjectsV1Removed)

	if hostVersionErr == nil && v1ProjectCutoffVersionErr == nil && hostVersion.LessThan(v1ProjectCutoffVersion) {
		return gh.ProjectsV1Supported
	}

	return gh.ProjectsV1Unsupported
}

func (d *detector) ProjectFeatures() (ProjectFeatures, error) {
	return allProjectFeatures, nil
}

func (d *detector) SearchFeatures() (SearchFeatures, error) {
	isEnterprise := ghauth.IsEnterprise(d.host)
	return SearchFeatures{
		AdvancedIssueSearchAPI:      true,
		AdvancedIssueSearchAPIOptIn: false,
		SemanticSearch:              !isEnterprise,
		HybridSearch:                !isEnterprise,
	}, nil
}

func (d *detector) ReleaseFeatures() (ReleaseFeatures, error) {
	return ReleaseFeatures{
		ImmutableReleases: true,
	}, nil
}

const (
	enterpriseWorkflowDispatchRunDetailsSupport = "3.21.0"
)

func (d *detector) ActionsFeatures() (ActionsFeatures, error) {
	// TODO workflowDispatchRunDetailsCleanup
	// Once GHES 3.20 support ends, we don't need feature detection for workflow dispatch (i.e. run details support).
	//
	// On github.com, workflow dispatch API now supports a new field named `return_run_details` that enabling it will
	// result in a 200 OK response with the details of the created workflow run. If not set (or set to false), the API
	// will keep the old behavior of returning a 204 No Content response.
	//
	// On GHES (current latest at 3.20), this new field is not available, and setting it will cause a 400 response.
	//
	// Once GHES 3.20 support ends, we can remove the feature detection and start using the new field in API calls.
	//
	// IMPORTANT: In the future REST API versions (i.e. breaking changes), the workflow dispatch endpoint is going to
	// always return the details of the created workflow run in the response, and the `return_run_details` field is
	// going to be ignored/removed. So, once we are migrating to the new API version we should double check the status
	// of the API.

	if !ghauth.IsEnterprise(d.host) {
		return ActionsFeatures{
			DispatchRunDetails: true,
		}, nil
	}

	minSupportedVersion, err := version.NewVersion(enterpriseWorkflowDispatchRunDetailsSupport)
	if err != nil {
		return ActionsFeatures{}, err
	}

	hostVersion, err := resolveEnterpriseVersion(d.httpClient, d.host)
	if err != nil {
		return ActionsFeatures{}, err
	}

	if hostVersion.GreaterThanOrEqual(minSupportedVersion) {
		return ActionsFeatures{
			DispatchRunDetails: true,
		}, nil
	}

	return ActionsFeatures{
		DispatchRunDetails: false,
	}, nil
}

// AuthFeatures checks the unauthenticated meta endpoint for GHES support.
func (d *detector) AuthFeatures() (AuthFeatures, error) {
	if !ghauth.IsEnterprise(d.host) {
		return AuthFeatures{ExpiringTokens: true}, nil
	}

	hostVersion, err := resolveEnterpriseVersion(d.httpClient, d.host)
	if err != nil {
		return AuthFeatures{}, err
	}

	return AuthFeatures{
		ExpiringTokens: hostVersion.GreaterThanOrEqual(version.Must(version.NewVersion("3.23.0"))),
	}, nil
}

func resolveEnterpriseVersion(httpClient *http.Client, host string) (*version.Version, error) {
	var metaResponse struct {
		InstalledVersion string `json:"installed_version"`
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	u, err := safeurl.JoinPath("meta")
	if err != nil {
		return nil, err
	}
	err = apiClient.REST(host, "GET", u.String(), nil, &metaResponse)
	if err != nil {
		return nil, err
	}

	return version.NewVersion(metaResponse.InstalledVersion)
}
