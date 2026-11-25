package queries

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func NewClient(httpClient *http.Client, hostname string, ios *iostreams.IOStreams) *Client {
	apiClient := &hostScopedClient{
		hostname: hostname,
		Client:   api.NewClientFromHTTP(httpClient),
	}
	return &Client{
		apiClient: apiClient,
		io:        ios,
		prompter:  prompter.New("", ios),
	}
}

// TestClientOpt is a test option for the test client.
type TestClientOpt func(*Client)

// WithPrompter is a test option to set the prompter for the test client.
func WithPrompter(p iprompter) TestClientOpt {
	return func(c *Client) {
		c.prompter = p
	}
}

func NewTestClient(opts ...TestClientOpt) *Client {
	apiClient := &hostScopedClient{
		hostname: "github.com",
		Client:   api.NewClientFromHTTP(http.DefaultClient),
	}
	io, _, _, _ := iostreams.Test()
	c := &Client{
		apiClient: apiClient,
		io:        io,
		prompter:  nil,
	}

	for _, o := range opts {
		o(c)
	}
	return c
}

type iprompter interface {
	Select(string, string, []string) (int, error)
}

type hostScopedClient struct {
	*api.Client
	hostname string
}

func (c *hostScopedClient) Query(_ string, _ any, _ map[string]any) error {
	return api.NotSupportedError("GitHub Projects is not supported with REST")
}

func (c *hostScopedClient) Mutate(_ string, _ any, _ map[string]any) error {
	return api.NotSupportedError("GitHub Projects is not supported with REST")
}

type graphqlClient interface {
	Query(queryName string, query any, variables map[string]any) error
	Mutate(queryName string, query any, variables map[string]any) error
}

type Client struct {
	apiClient graphqlClient
	io        *iostreams.IOStreams
	prompter  iprompter
}

const (
	LimitDefault = 30
	LimitMax     = 100 // https://docs.github.com/en/graphql/overview/resource-limitations#node-limit
)

// doQueryWithProgressIndicator wraps API calls with a progress indicator.
// The query name is used in the progress indicator label.
func (c *Client) doQueryWithProgressIndicator(name string, query any, variables map[string]any) error {
	c.io.StartProgressIndicatorWithLabel(fmt.Sprintf("Fetching %s", name))
	defer c.io.StopProgressIndicator()
	err := c.apiClient.Query(name, query, variables)
	return handleError(err)
}

// TODO: un-export this since it couples the caller heavily to api.GraphQLClient
func (c *Client) Mutate(operationName string, query any, variables map[string]any) error {
	err := c.apiClient.Mutate(operationName, query, variables)
	return handleError(err)
}

func (c *Client) Query(operationName string, query any, variables map[string]any) error {
	err := c.apiClient.Query(operationName, query, variables)
	return handleError(err)
}

// PageInfo is a PageInfo GraphQL object https://docs.github.com/en/graphql/reference/objects#pageinfo.
type PageInfo struct {
	EndCursor   string
	HasNextPage bool
}

// Project is a ProjectV2 GraphQL object https://docs.github.com/en/graphql/reference/objects#projectv2.
type Project struct {
	Number           int32
	URL              string
	ShortDescription string
	Public           bool
	Closed           bool
	// The Template field is commented out due to https://github.com/cli/cli/issues/8103.
	// We released gh v2.34.0 without realizing the Template field does not exist
	// on GHES 3.8 and older. This broke all project commands for users targeting GHES 3.8
	// and older. In order to fix this we will no longer query the Template field until
	// GHES 3.8 gets deprecated on 2024-03-07. This solution was simpler and quicker
	// than adding a feature detection measure to every place this query is used.
	// It does have the negative consequence that we have had to remove the
	// Template field when outputting projects to JSON using the --format flag supported
	// by a number of project commands. See `pkg/cmd/project/shared/format/json.go` for
	// implementation.
	// Template         bool
	Title  string
	ID     string
	Readme string
	Items  struct {
		PageInfo   PageInfo
		TotalCount int
		Nodes      []ProjectItem
	} `graphql:"items(first: $firstItems, after: $afterItems, query: $query)"`
	Fields ProjectFields `graphql:"fields(first: $firstFields, after: $afterFields)"`
	Owner  struct {
		TypeName string `graphql:"__typename"`
		User     struct {
			Login string
		} `graphql:"... on User"`
		Organization struct {
			Login string
		} `graphql:"... on Organization"`
	}
}

// ProjectMutationQuery is a ProjectV2 response shape for mutation payloads.
// It intentionally avoids the queryable items connection to prevent requiring a $query variable.
type ProjectMutationQuery struct {
	Number           int32
	URL              string
	ShortDescription string
	Public           bool
	Closed           bool
	Title            string
	ID               string
	Readme           string
	Items            struct {
		TotalCount int
	} `graphql:"items(first: $firstItems, after: $afterItems)"`
	Fields struct {
		TotalCount int
	} `graphql:"fields(first: $firstFields, after: $afterFields)"`
	Owner struct {
		TypeName string `graphql:"__typename"`
		User     struct {
			Login string
		} `graphql:"... on User"`
		Organization struct {
			Login string
		} `graphql:"... on Organization"`
	}
}

// Below, you will find the query structs to represent fetching a project via the GraphQL API.
// Prior to GHES 3.20, the query argument did not exist on the items connection, so we have
// one base struct and two structs that embed and add the Items connection with and without the query argument.
// The expectation is that these will then be converted into the Project domain struct above.
type projectQueryBase struct {
	Number           int32
	URL              string
	ShortDescription string
	Public           bool
	Closed           bool
	Title            string
	ID               string
	Readme           string
	Owner            struct {
		TypeName string `graphql:"__typename"`
		User     struct {
			Login string
		} `graphql:"... on User"`
		Organization struct {
			Login string
		} `graphql:"... on Organization"`
	}
	Fields ProjectFields `graphql:"fields(first: $firstFields, after: $afterFields)"`
}

type projectQueryWithQueryableItems struct {
	projectQueryBase
	Items struct {
		PageInfo   PageInfo
		TotalCount int
		Nodes      []ProjectItem
	} `graphql:"items(first: $firstItems, after: $afterItems, query: $query)"`
}

type projectQueryWithoutQueryableItems struct {
	projectQueryBase
	Items struct {
		PageInfo   PageInfo
		TotalCount int
		Nodes      []ProjectItem
	} `graphql:"items(first: $firstItems, after: $afterItems)"`
}

func newProjectFromQueryBase(source projectQueryBase) *Project {
	project := &Project{
		Number:           source.Number,
		URL:              source.URL,
		ShortDescription: source.ShortDescription,
		Public:           source.Public,
		Closed:           source.Closed,
		Title:            source.Title,
		ID:               source.ID,
		Readme:           source.Readme,
	}
	project.Owner.TypeName = source.Owner.TypeName
	project.Owner.User.Login = source.Owner.User.Login
	project.Owner.Organization.Login = source.Owner.Organization.Login
	project.Fields = source.Fields
	return project
}

func newProjectFromQueryWithItemsQuery(source projectQueryWithQueryableItems) *Project {
	project := newProjectFromQueryBase(source.projectQueryBase)
	project.Items.PageInfo = source.Items.PageInfo
	project.Items.TotalCount = source.Items.TotalCount
	project.Items.Nodes = source.Items.Nodes
	return project
}

func newProjectFromQueryWithoutItemsQuery(source projectQueryWithoutQueryableItems) *Project {
	project := newProjectFromQueryBase(source.projectQueryBase)
	project.Items.PageInfo = source.Items.PageInfo
	project.Items.TotalCount = source.Items.TotalCount
	project.Items.Nodes = source.Items.Nodes
	return project
}

func (p Project) DetailedItems() map[string]any {
	return map[string]any{
		"items":      serializeProjectWithItems(&p),
		"totalCount": p.Items.TotalCount,
	}
}

func (p Project) ExportData(_ []string) map[string]any {
	return map[string]any{
		"number":           p.Number,
		"url":              p.URL,
		"shortDescription": p.ShortDescription,
		"public":           p.Public,
		"closed":           p.Closed,
		"title":            p.Title,
		"id":               p.ID,
		"readme":           p.Readme,
		"items": map[string]any{
			"totalCount": p.Items.TotalCount,
		},
		"fields": map[string]any{
			"totalCount": p.Fields.TotalCount,
		},
		"owner": map[string]any{
			"type":  p.OwnerType(),
			"login": p.OwnerLogin(),
		},
	}
}

func (p Project) OwnerType() string {
	return p.Owner.TypeName
}

func (p Project) OwnerLogin() string {
	if p.OwnerType() == "User" {
		return p.Owner.User.Login
	}
	return p.Owner.Organization.Login
}

func (p ProjectMutationQuery) ExportData(_ []string) map[string]any {
	return map[string]any{
		"number":           p.Number,
		"url":              p.URL,
		"shortDescription": p.ShortDescription,
		"public":           p.Public,
		"closed":           p.Closed,
		"title":            p.Title,
		"id":               p.ID,
		"readme":           p.Readme,
		"items": map[string]any{
			"totalCount": p.Items.TotalCount,
		},
		"fields": map[string]any{
			"totalCount": p.Fields.TotalCount,
		},
		"owner": map[string]any{
			"type":  p.OwnerType(),
			"login": p.OwnerLogin(),
		},
	}
}

func (p ProjectMutationQuery) OwnerType() string {
	return p.Owner.TypeName
}

func (p ProjectMutationQuery) OwnerLogin() string {
	if p.OwnerType() == "User" {
		return p.Owner.User.Login
	}
	return p.Owner.Organization.Login
}

type Projects struct {
	Nodes      []Project
	TotalCount int
}

func (p Projects) ExportData(_ []string) map[string]any {
	v := make([]map[string]any, len(p.Nodes))
	for i := range p.Nodes {
		v[i] = p.Nodes[i].ExportData(nil)
	}
	return map[string]any{
		"projects":   v,
		"totalCount": p.TotalCount,
	}
}

// ProjectItem is a ProjectV2Item GraphQL object https://docs.github.com/en/graphql/reference/objects#projectv2item.
type ProjectItem struct {
	Content     ProjectItemContent
	Id          string
	FieldValues struct {
		Nodes []FieldValueNodes
	} `graphql:"fieldValues(first: 100)"` // hardcoded to 100 for now on the assumption that this is a reasonable limit
}

type ProjectItemContent struct {
	TypeName    string      `graphql:"__typename"`
	DraftIssue  DraftIssue  `graphql:"... on DraftIssue"`
	PullRequest PullRequest `graphql:"... on PullRequest"`
	Issue       Issue       `graphql:"... on Issue"`
}

type FieldValueNodes struct {
	Type                        string `graphql:"__typename"`
	ProjectV2ItemFieldDateValue struct {
		Date  string
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldDateValue"`
	ProjectV2ItemFieldIterationValue struct {
		Title       string
		StartDate   string
		Duration    int
		Field       ProjectField
		IterationId string
	} `graphql:"... on ProjectV2ItemFieldIterationValue"`
	ProjectV2ItemFieldLabelValue struct {
		Labels struct {
			Nodes []struct {
				Name string
			}
		} `graphql:"labels(first: 10)"` // experienced issues with larger limits, 10 seems like enough for now
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldLabelValue"`
	ProjectV2ItemFieldNumberValue struct {
		Number float64
		Field  ProjectField
	} `graphql:"... on ProjectV2ItemFieldNumberValue"`
	ProjectV2ItemFieldSingleSelectValue struct {
		Name  string
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
	ProjectV2ItemFieldTextValue struct {
		Text  string
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldTextValue"`
	ProjectV2ItemFieldMilestoneValue struct {
		Milestone struct {
			Title       string
			Description string
			DueOn       string
		}
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldMilestoneValue"`
	ProjectV2ItemFieldPullRequestValue struct {
		PullRequests struct {
			Nodes []struct {
				Url string
			}
		} `graphql:"pullRequests(first:10)"` // experienced issues with larger limits, 10 seems like enough for now
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldPullRequestValue"`
	ProjectV2ItemFieldRepositoryValue struct {
		Repository struct {
			Url string
		}
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldRepositoryValue"`
	ProjectV2ItemFieldUserValue struct {
		Users struct {
			Nodes []struct {
				Login string
			}
		} `graphql:"users(first: 10)"` // experienced issues with larger limits, 10 seems like enough for now
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldUserValue"`
	ProjectV2ItemFieldReviewerValue struct {
		Reviewers struct {
			Nodes []struct {
				Type string `graphql:"__typename"`
				Team struct {
					Name string
				} `graphql:"... on Team"`
				User struct {
					Login string
				} `graphql:"... on User"`
			}
		} `graphql:"reviewers(first: 10)"` // experienced issues with larger limits, 10 seems like enough for now
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldReviewerValue"`
}

func (v FieldValueNodes) ID() string {
	switch v.Type {
	case "ProjectV2ItemFieldDateValue":
		return v.ProjectV2ItemFieldDateValue.Field.ID()
	case "ProjectV2ItemFieldIterationValue":
		return v.ProjectV2ItemFieldIterationValue.Field.ID()
	case "ProjectV2ItemFieldNumberValue":
		return v.ProjectV2ItemFieldNumberValue.Field.ID()
	case "ProjectV2ItemFieldSingleSelectValue":
		return v.ProjectV2ItemFieldSingleSelectValue.Field.ID()
	case "ProjectV2ItemFieldTextValue":
		return v.ProjectV2ItemFieldTextValue.Field.ID()
	case "ProjectV2ItemFieldMilestoneValue":
		return v.ProjectV2ItemFieldMilestoneValue.Field.ID()
	case "ProjectV2ItemFieldLabelValue":
		return v.ProjectV2ItemFieldLabelValue.Field.ID()
	case "ProjectV2ItemFieldPullRequestValue":
		return v.ProjectV2ItemFieldPullRequestValue.Field.ID()
	case "ProjectV2ItemFieldRepositoryValue":
		return v.ProjectV2ItemFieldRepositoryValue.Field.ID()
	case "ProjectV2ItemFieldUserValue":
		return v.ProjectV2ItemFieldUserValue.Field.ID()
	case "ProjectV2ItemFieldReviewerValue":
		return v.ProjectV2ItemFieldReviewerValue.Field.ID()
	}

	return ""
}

// DisplayValue returns a single-line, human-readable rendering of the field
// value suitable for a table cell. Multi-value fields (labels, assignees, etc.)
// are joined with commas, and embedded line breaks are collapsed so a value with
// newlines cannot corrupt table row alignment. It is used by item-list to show a
// named field's value without a separate field-ID preflight lookup.
func (v FieldValueNodes) DisplayValue() string {
	var value string
	switch data := projectFieldValueData(v).(type) {
	case nil:
		return ""
	case string:
		value = data
	case float64:
		value = strconv.FormatFloat(data, 'f', -1, 64)
	case []string:
		value = strings.Join(data, ", ")
	case map[string]any:
		title, _ := data["title"].(string)
		value = title
	default:
		value = fmt.Sprintf("%v", data)
	}
	return singleLineFieldValue(value)
}

// singleLineFieldValue collapses carriage returns and newlines so a multi-line
// field value stays within a single table cell. Following the convention used by
// StatusItem.Preview, carriage returns are dropped and newlines become spaces.
func singleLineFieldValue(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", " ")
}

type DraftIssue struct {
	ID    string
	Body  string
	Title string
}

func (i DraftIssue) ExportData(_ []string) map[string]any {
	v := map[string]any{
		"title": i.Title,
		"body":  i.Body,
		"type":  "DraftIssue",
	}
	// Emulate omitempty.
	if i.ID != "" {
		v["id"] = i.ID
	}
	return v
}

type PullRequest struct {
	Body       string
	Title      string
	Number     int
	URL        string
	Repository struct {
		NameWithOwner string
	}
}

func (pr PullRequest) ExportData(_ []string) map[string]any {
	return map[string]any{
		"type":       "PullRequest",
		"body":       pr.Body,
		"title":      pr.Title,
		"number":     pr.Number,
		"repository": pr.Repository.NameWithOwner,
		"url":        pr.URL,
	}
}

type Issue struct {
	Body       string
	Title      string
	Number     int
	URL        string
	Repository struct {
		NameWithOwner string
	}
}

func (i Issue) ExportData(_ []string) map[string]any {
	return map[string]any{
		"type":       "Issue",
		"body":       i.Body,
		"title":      i.Title,
		"number":     i.Number,
		"repository": i.Repository.NameWithOwner,
		"url":        i.URL,
	}
}

func (p ProjectItem) DetailedItem() exportable {
	switch p.Type() {
	case "DraftIssue":
		return DraftIssue{
			ID:    p.Content.DraftIssue.ID,
			Body:  p.Body(),
			Title: p.Title(),
		}

	case "Issue":
		return Issue{
			Body:   p.Body(),
			Title:  p.Title(),
			Number: p.Number(),
			Repository: struct{ NameWithOwner string }{
				NameWithOwner: p.Repo(),
			},
			URL: p.URL(),
		}

	case "PullRequest":
		return PullRequest{
			Body:   p.Body(),
			Title:  p.Title(),
			Number: p.Number(),
			Repository: struct{ NameWithOwner string }{
				NameWithOwner: p.Repo(),
			},
			URL: p.URL(),
		}
	}
	return nil
}

// Type is the underlying type of the project item.
func (p ProjectItem) Type() string {
	return p.Content.TypeName
}

// Title is the title of the project item.
func (p ProjectItem) Title() string {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.Title
	case "PullRequest":
		return p.Content.PullRequest.Title
	case "DraftIssue":
		return p.Content.DraftIssue.Title
	}
	return ""
}

// Body is the body of the project item.
func (p ProjectItem) Body() string {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.Body
	case "PullRequest":
		return p.Content.PullRequest.Body
	case "DraftIssue":
		return p.Content.DraftIssue.Body
	}
	return ""
}

// Number is the number of the project item. It is only valid for issues and pull requests.
func (p ProjectItem) Number() int {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.Number
	case "PullRequest":
		return p.Content.PullRequest.Number
	}

	return 0
}

// ID is the id of the ProjectItem.
func (p ProjectItem) ID() string {
	return p.Id
}

// Repo is the repository of the project item. It is only valid for issues and pull requests.
func (p ProjectItem) Repo() string {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.Repository.NameWithOwner
	case "PullRequest":
		return p.Content.PullRequest.Repository.NameWithOwner
	}
	return ""
}

// URL is the URL of the project item. Note the draft issues do not have URLs
func (p ProjectItem) URL() string {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.URL
	case "PullRequest":
		return p.Content.PullRequest.URL
	}
	return ""
}

func (p ProjectItem) ExportData(_ []string) map[string]any {
	v := map[string]any{
		"id":    p.ID(),
		"title": p.Title(),
		"body":  p.Body(),
		"type":  p.Type(),
	}
	// Emulate omitempty.
	if url := p.URL(); url != "" {
		v["url"] = url
	}
	return v
}

// ProjectItems returns the items of a project. If the OwnerType is VIEWER, no login is required.
// If limit is 0, the default limit is used. The queryStr parameter is passed as a server-side
// filter to the items connection, using the same syntax as the GitHub Projects filter bar
// (e.g. "assignee:octocat", "status:done").
func (c *Client) ProjectItems(_ *Owner, _ int32, _ int, _ string) (*Project, error) {
	return &Project{}, api.NotSupportedError("GitHub Projects is not supported with REST")
}

// pager is an interface for paginating over the attributes of a Project.
type pager[N projectAttribute] interface {
	HasNextPage() bool
	EndCursor() string
	Nodes() []N
	Project() *Project
}

// userOwnerWithItemsNoQuery
func (q userOwnerWithItemsNoQuery) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q userOwnerWithItemsNoQuery) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q userOwnerWithItemsNoQuery) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q userOwnerWithItemsNoQuery) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// userOwnerWithItems
func (q userOwnerWithItems) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q userOwnerWithItems) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q userOwnerWithItems) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q userOwnerWithItems) Project() *Project {
	return newProjectFromQueryWithItemsQuery(q.Owner.Project)
}

// orgOwnerWithItems
func (q orgOwnerWithItems) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q orgOwnerWithItems) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q orgOwnerWithItems) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q orgOwnerWithItems) Project() *Project {
	return newProjectFromQueryWithItemsQuery(q.Owner.Project)
}

// orgOwnerWithItemsNoQuery
func (q orgOwnerWithItemsNoQuery) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q orgOwnerWithItemsNoQuery) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q orgOwnerWithItemsNoQuery) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q orgOwnerWithItemsNoQuery) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// viewerOwnerWithItems
func (q viewerOwnerWithItems) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q viewerOwnerWithItems) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q viewerOwnerWithItems) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q viewerOwnerWithItems) Project() *Project {
	return newProjectFromQueryWithItemsQuery(q.Owner.Project)
}

// viewerOwnerWithItemsNoQuery
func (q viewerOwnerWithItemsNoQuery) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q viewerOwnerWithItemsNoQuery) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q viewerOwnerWithItemsNoQuery) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q viewerOwnerWithItemsNoQuery) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// userOwnerWithFields
func (q userOwnerWithFields) HasNextPage() bool {
	return q.Owner.Project.Fields.PageInfo.HasNextPage
}

func (q userOwnerWithFields) EndCursor() string {
	return string(q.Owner.Project.Fields.PageInfo.EndCursor)
}

func (q userOwnerWithFields) Nodes() []ProjectField {
	return q.Owner.Project.Fields.Nodes
}

func (q userOwnerWithFields) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// orgOwnerWithFields
func (q orgOwnerWithFields) HasNextPage() bool {
	return q.Owner.Project.Fields.PageInfo.HasNextPage
}

func (q orgOwnerWithFields) EndCursor() string {
	return string(q.Owner.Project.Fields.PageInfo.EndCursor)
}

func (q orgOwnerWithFields) Nodes() []ProjectField {
	return q.Owner.Project.Fields.Nodes
}

func (q orgOwnerWithFields) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// viewerOwnerWithFields
func (q viewerOwnerWithFields) HasNextPage() bool {
	return q.Owner.Project.Fields.PageInfo.HasNextPage
}

func (q viewerOwnerWithFields) EndCursor() string {
	return string(q.Owner.Project.Fields.PageInfo.EndCursor)
}

func (q viewerOwnerWithFields) Nodes() []ProjectField {
	return q.Owner.Project.Fields.Nodes
}

func (q viewerOwnerWithFields) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

type projectAttribute interface {
	ProjectItem | ProjectField
}

// paginateAttributes is for paginating over the attributes of a project, such as items or fields
//
// firstKey and afterKey are the keys in the variables map that are used to set the first and after
// as these are set independently based on the attribute type, such as item or field.
//
// limit is the maximum number of attributes to return, or 0 for no limit.
//
// nodes is the list of attributes that have already been fetched.
//
// the return value is a slice of the newly fetched attributes appended to nodes.
func paginateAttributes[N projectAttribute](c *Client, p pager[N], variables map[string]any, queryName string, firstKey string, afterKey string, limit int, nodes []N) ([]N, error) {
	hasNextPage := p.HasNextPage()
	cursor := p.EndCursor()
	for {
		if !hasNextPage || len(nodes) >= limit {
			return nodes, nil
		}

		if len(nodes)+LimitMax > limit {
			first := limit - len(nodes)
			variables[firstKey] = first
		}
		variables[afterKey] = cursor
		err := c.doQueryWithProgressIndicator(queryName, p, variables)
		if err != nil {
			return nodes, err
		}

		nodes = append(nodes, p.Nodes()...)
		hasNextPage = p.HasNextPage()
		cursor = p.EndCursor()
	}
}

// ProjectField is a ProjectV2FieldConfiguration GraphQL object https://docs.github.com/en/graphql/reference/unions#projectv2fieldconfiguration.
type ProjectField struct {
	TypeName string `graphql:"__typename"`
	Field    struct {
		ID       string
		Name     string
		DataType string
	} `graphql:"... on ProjectV2Field"`
	IterationField struct {
		ID       string
		Name     string
		DataType string
	} `graphql:"... on ProjectV2IterationField"`
	SingleSelectField struct {
		ID       string
		Name     string
		DataType string
		Options  []SingleSelectFieldOptions
	} `graphql:"... on ProjectV2SingleSelectField"`
}

// ID is the ID of the project field.
func (p ProjectField) ID() string {
	if p.TypeName == "ProjectV2Field" {
		return p.Field.ID
	} else if p.TypeName == "ProjectV2IterationField" {
		return p.IterationField.ID
	} else if p.TypeName == "ProjectV2SingleSelectField" {
		return p.SingleSelectField.ID
	}
	return ""
}

// Name is the name of the project field.
func (p ProjectField) Name() string {
	if p.TypeName == "ProjectV2Field" {
		return p.Field.Name
	} else if p.TypeName == "ProjectV2IterationField" {
		return p.IterationField.Name
	} else if p.TypeName == "ProjectV2SingleSelectField" {
		return p.SingleSelectField.Name
	}
	return ""
}

// Type is the typename of the project field.
func (p ProjectField) Type() string {
	return p.TypeName
}

// DataType is the data type of the project field, e.g. TEXT, NUMBER, DATE,
// SINGLE_SELECT, or ITERATION. It returns an empty string for unknown field types.
func (p ProjectField) DataType() string {
	switch p.TypeName {
	case "ProjectV2Field":
		return p.Field.DataType
	case "ProjectV2IterationField":
		return p.IterationField.DataType
	case "ProjectV2SingleSelectField":
		return p.SingleSelectField.DataType
	}
	return ""
}

type SingleSelectFieldOptions struct {
	ID   string
	Name string
}

func (f SingleSelectFieldOptions) ExportData(_ []string) map[string]any {
	return map[string]any{
		"id":   f.ID,
		"name": f.Name,
	}
}

func (p ProjectField) Options() []SingleSelectFieldOptions {
	if p.TypeName == "ProjectV2SingleSelectField" {
		var options []SingleSelectFieldOptions
		for _, o := range p.SingleSelectField.Options {
			options = append(options, SingleSelectFieldOptions{
				ID:   o.ID,
				Name: o.Name,
			})
		}
		return options
	}
	return nil
}

func (p ProjectField) ExportData(_ []string) map[string]any {
	v := map[string]any{
		"id":   p.ID(),
		"name": p.Name(),
		"type": p.Type(),
	}
	// Emulate omitempty
	if opts := p.Options(); len(opts) != 0 {
		options := make([]map[string]any, len(opts))
		for i, opt := range opts {
			options[i] = opt.ExportData(nil)
		}
		v["options"] = options
	}
	return v
}

type ProjectFields struct {
	TotalCount int
	Nodes      []ProjectField
	PageInfo   PageInfo
}

func (p ProjectFields) ExportData(_ []string) map[string]any {
	fields := make([]map[string]any, len(p.Nodes))
	for i := range p.Nodes {
		fields[i] = p.Nodes[i].ExportData(nil)
	}
	return map[string]any{
		"fields":     fields,
		"totalCount": p.TotalCount,
	}
}

// ProjectFields returns a project with fields. If the OwnerType is VIEWER, no login is required.
// If limit is 0, the default limit is used.
func (c *Client) ProjectFields(_ *Owner, _ int32, _ int) (*Project, error) {
	return &Project{}, api.NotSupportedError("GitHub Projects is not supported with REST")
}

// viewerLogin is used to query the Login of the viewer.
type viewerLogin struct {
	Viewer struct {
		Login string
		Id    string
	}
}

type viewerLoginOrgs struct {
	Viewer struct {
		Login         string
		ID            string
		Organizations struct {
			PageInfo PageInfo
			Nodes    []struct {
				Login                   string
				ViewerCanCreateProjects bool
				ID                      string
			}
		} `graphql:"organizations(first: 100, after: $after)"`
	}
}

type ownerWithLogin struct {
	Project projectQueryWithoutQueryableItems `graphql:"projectV2(number: $number)"`
	Login   string
}

type ownerWithProjectWithItemQuery struct {
	Project projectQueryWithQueryableItems `graphql:"projectV2(number: $number)"`
}

type ownerWithProjectWithoutItemQuery struct {
	Project projectQueryWithoutQueryableItems `graphql:"projectV2(number: $number)"`
}

// userOwner is used to query the project of a user.
type userOwner struct {
	Owner ownerWithLogin `graphql:"user(login: $login)"`
}

// userOwnerWithItems is used to query the project of a user with its items.
type userOwnerWithItems struct {
	Owner ownerWithProjectWithItemQuery `graphql:"user(login: $login)"`
}

// userOwnerWithItemsNoQuery is used to query the project of a user with its items, without query support.
type userOwnerWithItemsNoQuery struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"user(login: $login)"`
}

// userOwnerWithFields is used to query the project of a user with its fields.
type userOwnerWithFields struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"user(login: $login)"`
}

// orgOwner is used to query the project of an organization.
type orgOwner struct {
	Owner ownerWithLogin `graphql:"organization(login: $login)"`
}

// orgOwnerWithItems is used to query the project of an organization with its items.
type orgOwnerWithItems struct {
	Owner ownerWithProjectWithItemQuery `graphql:"organization(login: $login)"`
}

// orgOwnerWithItemsNoQuery is used to query the project of an organization with its items, without query support.
type orgOwnerWithItemsNoQuery struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"organization(login: $login)"`
}

// orgOwnerWithFields is used to query the project of an organization with its fields.
type orgOwnerWithFields struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"organization(login: $login)"`
}

// viewerOwner is used to query the project of the viewer.
type viewerOwner struct {
	Owner ownerWithLogin `graphql:"viewer"`
}

// viewerOwnerWithItems is used to query the project of the viewer with its items.
type viewerOwnerWithItems struct {
	Owner ownerWithProjectWithItemQuery `graphql:"viewer"`
}

// viewerOwnerWithItemsNoQuery is used to query the project of the viewer with its items, without query support.
type viewerOwnerWithItemsNoQuery struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"viewer"`
}

// viewerOwnerWithFields is used to query the project of the viewer with its fields.
type viewerOwnerWithFields struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"viewer"`
}

// OwnerType is the type of the owner of a project, which can be either a user or an organization. Viewer is the current user.
type OwnerType string

const UserOwner OwnerType = "USER"
const OrgOwner OwnerType = "ORGANIZATION"
const ViewerOwner OwnerType = "VIEWER"

// ViewerLoginName returns the login name of the viewer.
func (c *Client) ViewerLoginName() (string, error) {
	var query viewerLogin
	err := c.doQueryWithProgressIndicator("Viewer", &query, map[string]any{})
	if err != nil {
		return "", err
	}
	return query.Viewer.Login, nil
}

// OwnerIDAndType returns NotSupportedError because project owners use GraphQL node IDs.
func (c *Client) OwnerIDAndType(_ string) (string, OwnerType, error) {
	return "", "", api.NotSupportedError("GitHub Projects is not supported with REST")
}

// issueOrPullRequest is used to query the global id of an issue or pull request by its URL.
type issueOrPullRequest struct {
	Resource struct {
		Typename string `graphql:"__typename"`
		Issue    struct {
			ID string
		} `graphql:"... on Issue"`
		PullRequest struct {
			ID string
		} `graphql:"... on PullRequest"`
	} `graphql:"resource(url: $url)"`
}

// IssueOrPullRequestID returns NotSupportedError because GitHub Projects is not supported with REST.
func (c *Client) IssueOrPullRequestID(_ string) (string, error) {
	return "", api.NotSupportedError("GitHub Projects is not supported with REST")
}

// userProjects queries the $first projects of a user.
type userProjects struct {
	Owner struct {
		Projects struct {
			TotalCount int
			PageInfo   PageInfo
			Nodes      []projectQueryWithoutQueryableItems
		} `graphql:"projectsV2(first: $first, after: $after)"`
		Login string
	} `graphql:"user(login: $login)"`
}

// orgProjects queries the $first projects of an organization.
type orgProjects struct {
	Owner struct {
		Projects struct {
			TotalCount int
			PageInfo   PageInfo
			Nodes      []projectQueryWithoutQueryableItems
		} `graphql:"projectsV2(first: $first, after: $after)"`
		Login string
	} `graphql:"organization(login: $login)"`
}

// viewerProjects queries the $first projects of the viewer.
type viewerProjects struct {
	Owner struct {
		Projects struct {
			TotalCount int
			PageInfo   PageInfo
			Nodes      []projectQueryWithoutQueryableItems
		} `graphql:"projectsV2(first: $first, after: $after)"`
		Login string
	} `graphql:"viewer"`
}

type loginTypes struct {
	Login string
	Type  OwnerType
	ID    string
}

func (c *Client) userOrgLogins() ([]loginTypes, error) {
	return nil, api.NotSupportedError("GitHub Projects is not supported with REST")
}

func (c *Client) paginateOrgLogins(l []loginTypes, _ string) ([]loginTypes, error) {
	return l, api.NotSupportedError("GitHub Projects is not supported with REST")
}

type Owner struct {
	Login string
	Type  OwnerType
	ID    string
}

// NewOwner creates a project Owner
// If canPrompt is false, login is required as we cannot prompt for it.
// If login is not empty, it is used to lookup the project owner.
// If login is empty, interactive mode is used to select an owner.
// from the current viewer and their organizations
func (c *Client) NewOwner(canPrompt bool, login string) (*Owner, error) {
	if login != "" {
		id, ownerType, err := c.OwnerIDAndType(login)
		if err != nil {
			return nil, err
		}

		return &Owner{
			Login: login,
			Type:  ownerType,
			ID:    id,
		}, nil
	}

	if !canPrompt {
		return nil, fmt.Errorf("owner is required when not running interactively")
	}

	logins, err := c.userOrgLogins()
	if err != nil {
		return nil, err
	}

	options := make([]string, 0, len(logins))
	for _, l := range logins {
		options = append(options, l.Login)
	}

	answerIndex, err := c.prompter.Select("Which owner would you like to use?", "", options)
	if err != nil {
		return nil, err
	}

	l := logins[answerIndex]
	return &Owner{
		Login: l.Login,
		Type:  l.Type,
		ID:    l.ID,
	}, nil
}

// NewProject creates a project based on the owner and project number
// if canPrompt is false, number is required as we cannot prompt for it
// if number is 0 it will prompt the user to select a project interactively
// otherwise it will make a request to get the project by number
// set `fields“ to true to get the project's field data
func (c *Client) NewProject(canPrompt bool, _ *Owner, number int32, _ bool) (*Project, error) {
	if number == 0 && !canPrompt {
		return nil, fmt.Errorf("project number is required when not running interactively")
	}
	return nil, api.NotSupportedError("GitHub Projects is not supported with REST")
}

// Projects returns all the projects for an Owner. If the OwnerType is VIEWER, no login is required.
// If limit is 0, the default limit is used.
func (c *Client) Projects(_ string, _ OwnerType, _ int, _ bool) (Projects, error) {
	return Projects{Nodes: make([]Project, 0)}, api.NotSupportedError("GitHub Projects is not supported with REST")
}

type linkProjectToRepoMutation struct {
	LinkProjectV2ToRepository struct {
		ClientMutationId string `graphql:"clientMutationId"`
	} `graphql:"linkProjectV2ToRepository(input:$input)"`
}

type linkProjectToTeamMutation struct {
	LinkProjectV2ToTeam struct {
		ClientMutationId string `graphql:"clientMutationId"`
	} `graphql:"linkProjectV2ToTeam(input:$input)"`
}

type unlinkProjectFromRepoMutation struct {
	UnlinkProjectV2FromRepository struct {
		ClientMutationId string `graphql:"clientMutationId"`
	} `graphql:"unlinkProjectV2FromRepository(input:$input)"`
}

type unlinkProjectFromTeamMutation struct {
	UnlinkProjectV2FromTeam struct {
		ClientMutationId string `graphql:"clientMutationId"`
	} `graphql:"unlinkProjectV2FromTeam(input:$input)"`
}

// LinkProjectToRepository links a project to a repository.
func (c *Client) LinkProjectToRepository(_, _ string) error {
	return api.NotSupportedError("GitHub Projects is not supported with REST")
}

// LinkProjectToTeam links a project to a team.
func (c *Client) LinkProjectToTeam(_, _ string) error {
	return api.NotSupportedError("GitHub Projects is not supported with REST")
}

// UnlinkProjectFromRepository unlinks a project from a repository.
func (c *Client) UnlinkProjectFromRepository(_, _ string) error {
	return api.NotSupportedError("GitHub Projects is not supported with REST")
}

// UnlinkProjectFromTeam unlinks a project from a team.
func (c *Client) UnlinkProjectFromTeam(_, _ string) error {
	return api.NotSupportedError("GitHub Projects is not supported with REST")
}

func handleError(err error) error {
	return err
}

var scopesRE = regexp.MustCompile(`one of the following scopes: \[(.+?)]`)

func requiredScopesFromServerMessage(msg string) []string {
	m := scopesRE.FindStringSubmatch(msg)
	if m == nil {
		return nil
	}
	var scopes []string
	for mm := range strings.SplitSeq(m[1], ",") {
		scopes = append(scopes, strings.Trim(mm, "' "))
	}
	return scopes
}

func projectFieldValueData(v FieldValueNodes) any {
	switch v.Type {
	case "ProjectV2ItemFieldDateValue":
		return v.ProjectV2ItemFieldDateValue.Date
	case "ProjectV2ItemFieldIterationValue":
		return map[string]any{
			"title":       v.ProjectV2ItemFieldIterationValue.Title,
			"startDate":   v.ProjectV2ItemFieldIterationValue.StartDate,
			"duration":    v.ProjectV2ItemFieldIterationValue.Duration,
			"iterationId": v.ProjectV2ItemFieldIterationValue.IterationId,
		}
	case "ProjectV2ItemFieldNumberValue":
		return v.ProjectV2ItemFieldNumberValue.Number
	case "ProjectV2ItemFieldSingleSelectValue":
		return v.ProjectV2ItemFieldSingleSelectValue.Name
	case "ProjectV2ItemFieldTextValue":
		return v.ProjectV2ItemFieldTextValue.Text
	case "ProjectV2ItemFieldMilestoneValue":
		return map[string]any{
			"title":       v.ProjectV2ItemFieldMilestoneValue.Milestone.Title,
			"description": v.ProjectV2ItemFieldMilestoneValue.Milestone.Description,
			"dueOn":       v.ProjectV2ItemFieldMilestoneValue.Milestone.DueOn,
		}
	case "ProjectV2ItemFieldLabelValue":
		names := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldLabelValue.Labels.Nodes {
			names = append(names, p.Name)
		}
		return names
	case "ProjectV2ItemFieldPullRequestValue":
		urls := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldPullRequestValue.PullRequests.Nodes {
			urls = append(urls, p.Url)
		}
		return urls
	case "ProjectV2ItemFieldRepositoryValue":
		return v.ProjectV2ItemFieldRepositoryValue.Repository.Url
	case "ProjectV2ItemFieldUserValue":
		logins := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldUserValue.Users.Nodes {
			logins = append(logins, p.Login)
		}
		return logins
	case "ProjectV2ItemFieldReviewerValue":
		names := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldReviewerValue.Reviewers.Nodes {
			if p.Type == "Team" {
				names = append(names, p.Team.Name)
			} else if p.Type == "User" {
				names = append(names, p.User.Login)
			}
		}
		return names

	}

	return nil
}

// serialize creates a map from field to field values
func serializeProjectWithItems(project *Project) []map[string]any {
	fields := make(map[string]string)

	// make a map of fields by ID
	for _, f := range project.Fields.Nodes {
		fields[f.ID()] = camelCase(f.Name())
	}
	itemsSlice := make([]map[string]any, 0)

	// for each value, look up the name by ID
	// and set the value to the field value
	for _, i := range project.Items.Nodes {
		o := make(map[string]any)
		o["id"] = i.Id
		if projectItem := i.DetailedItem(); projectItem != nil {
			o["content"] = projectItem.ExportData(nil)
		} else {
			o["content"] = nil
		}
		for _, v := range i.FieldValues.Nodes {
			id := v.ID()
			value := projectFieldValueData(v)

			o[fields[id]] = value
		}
		itemsSlice = append(itemsSlice, o)
	}
	return itemsSlice
}

// camelCase converts a string to camelCase, which is useful for turning Go field names to JSON keys.
func camelCase(s string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) == 1 {
		return strings.ToLower(s)
	}
	return strings.ToLower(s[0:1]) + s[1:]
}

type exportable interface {
	ExportData([]string) map[string]any
}
