package queries

import (
	"reflect"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectQueries_NotSupported(t *testing.T) {
	client := NewTestClient()
	owner := &Owner{
		Type:  UserOwner,
		Login: "monalisa",
		ID:    "user ID",
	}

	err := client.Mutate("UpdateProjectV2", &struct{}{}, map[string]any{})
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = client.ProjectItems(owner, 1, LimitMax, "")
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = client.Projects("", ViewerOwner, 1, false)
	require.ErrorIs(t, err, api.ErrNotSupported)

	_, err = client.ProjectFields(owner, 1, LimitMax)
	require.ErrorIs(t, err, api.ErrNotSupported)
}





func Test_requiredScopesFromServerMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want []string
	}{
		{
			name: "no scopes",
			msg:  "SERVER OOPSIE",
			want: []string(nil),
		},
		{
			name: "one scope",
			msg:  "Your token has not been granted the required scopes to execute this query. The 'dataType' field requires one of the following scopes: ['read:project'], but your token has only been granted the: ['codespace', repo'] scopes. Please modify your token's scopes at: https://github.com/settings/tokens.",
			want: []string{"read:project"},
		},
		{
			name: "multiple scopes",
			msg:  "Your token has not been granted the required scopes to execute this query. The 'dataType' field requires one of the following scopes: ['read:project', 'read:discussion', 'codespace'], but your token has only been granted the: [repo'] scopes. Please modify your token's scopes at: https://github.com/settings/tokens.",
			want: []string{"read:project", "read:discussion", "codespace"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiredScopesFromServerMessage(tt.msg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("requiredScopesFromServerMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewProject_nonTTY(t *testing.T) {
	client := NewTestClient()
	_, err := client.NewProject(false, &Owner{}, 0, false)
	assert.EqualError(t, err, "project number is required when not running interactively")
}

func TestNewOwner_nonTTY(t *testing.T) {
	client := NewTestClient()
	_, err := client.NewOwner(false, "")
	assert.EqualError(t, err, "owner is required when not running interactively")

}



func TestCamelCase(t *testing.T) {
	assert.Equal(t, "camelCase", camelCase("camelCase"))
	assert.Equal(t, "camelCase", camelCase("CamelCase"))
	assert.Equal(t, "c", camelCase("C"))
	assert.Equal(t, "", camelCase(""))
}

func TestFieldValueNodesDisplayValue(t *testing.T) {
	textValue := func(text string) FieldValueNodes {
		v := FieldValueNodes{Type: "ProjectV2ItemFieldTextValue"}
		v.ProjectV2ItemFieldTextValue.Text = text
		return v
	}

	numberValue := FieldValueNodes{Type: "ProjectV2ItemFieldNumberValue"}
	numberValue.ProjectV2ItemFieldNumberValue.Number = 12

	singleSelectValue := FieldValueNodes{Type: "ProjectV2ItemFieldSingleSelectValue"}
	singleSelectValue.ProjectV2ItemFieldSingleSelectValue.Name = "In Progress"

	labelValue := FieldValueNodes{Type: "ProjectV2ItemFieldLabelValue"}
	labelValue.ProjectV2ItemFieldLabelValue.Labels.Nodes = []struct {
		Name string
	}{
		{Name: "bug"},
		{Name: "help wanted"},
	}

	dateValue := FieldValueNodes{Type: "ProjectV2ItemFieldDateValue"}
	dateValue.ProjectV2ItemFieldDateValue.Date = "2022-05-01"

	iterationValue := FieldValueNodes{Type: "ProjectV2ItemFieldIterationValue"}
	iterationValue.ProjectV2ItemFieldIterationValue.Title = "Sprint 1"

	milestoneValue := FieldValueNodes{Type: "ProjectV2ItemFieldMilestoneValue"}
	milestoneValue.ProjectV2ItemFieldMilestoneValue.Milestone.Title = "v1.0"

	pullRequestValue := FieldValueNodes{Type: "ProjectV2ItemFieldPullRequestValue"}
	pullRequestValue.ProjectV2ItemFieldPullRequestValue.PullRequests.Nodes = []struct {
		Url string
	}{
		{Url: "https://github.com/cli/cli/pull/1"},
		{Url: "https://github.com/cli/cli/pull/2"},
	}

	repositoryValue := FieldValueNodes{Type: "ProjectV2ItemFieldRepositoryValue"}
	repositoryValue.ProjectV2ItemFieldRepositoryValue.Repository.Url = "https://github.com/cli/cli"

	userValue := FieldValueNodes{Type: "ProjectV2ItemFieldUserValue"}
	userValue.ProjectV2ItemFieldUserValue.Users.Nodes = []struct {
		Login string
	}{
		{Login: "monalisa"},
		{Login: "hubot"},
	}

	reviewerValue := FieldValueNodes{Type: "ProjectV2ItemFieldReviewerValue"}
	reviewerValue.ProjectV2ItemFieldReviewerValue.Reviewers.Nodes = []struct {
		Type string `graphql:"__typename"`
		Team struct {
			Name string
		} `graphql:"... on Team"`
		User struct {
			Login string
		} `graphql:"... on User"`
	}{
		{
			Type: "User",
			User: struct {
				Login string
			}{Login: "monalisa"},
		},
		{
			Type: "Team",
			Team: struct {
				Name string
			}{Name: "octocat-team"},
		},
	}

	tests := []struct {
		name  string
		value FieldValueNodes
		want  string
	}{
		{
			name:  "empty when field has no value",
			value: FieldValueNodes{Type: "ProjectV2ItemFieldTextValue"},
			want:  "",
		},
		{
			name:  "unknown field type",
			value: FieldValueNodes{Type: "SomethingElse"},
			want:  "",
		},
		{
			name:  "single-line text",
			value: textValue("hello world"),
			want:  "hello world",
		},
		{
			name:  "multi-line text collapses newlines to spaces",
			value: textValue("first line\nsecond line"),
			want:  "first line second line",
		},
		{
			name:  "CRLF text collapses to a single space",
			value: textValue("first line\r\nsecond line"),
			want:  "first line second line",
		},
		{
			name:  "leading and trailing newlines",
			value: textValue("\nwrapped\n"),
			want:  " wrapped ",
		},
		{
			name:  "number",
			value: numberValue,
			want:  "12",
		},
		{
			name:  "single select",
			value: singleSelectValue,
			want:  "In Progress",
		},
		{
			name:  "labels joined with commas",
			value: labelValue,
			want:  "bug, help wanted",
		},
		{
			name:  "date",
			value: dateValue,
			want:  "2022-05-01",
		},
		{
			name:  "iteration title",
			value: iterationValue,
			want:  "Sprint 1",
		},
		{
			name:  "milestone title",
			value: milestoneValue,
			want:  "v1.0",
		},
		{
			name:  "pull requests joined with commas",
			value: pullRequestValue,
			want:  "https://github.com/cli/cli/pull/1, https://github.com/cli/cli/pull/2",
		},
		{
			name:  "repository url",
			value: repositoryValue,
			want:  "https://github.com/cli/cli",
		},
		{
			name:  "users joined with commas",
			value: userValue,
			want:  "monalisa, hubot",
		},
		{
			name:  "reviewers joined with commas for users and teams",
			value: reviewerValue,
			want:  "monalisa, octocat-team",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.value.DisplayValue())
		})
	}
}

func TestSingleLineFieldValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no line breaks", in: "plain", want: "plain"},
		{name: "newline becomes space", in: "a\nb", want: "a b"},
		{name: "CRLF becomes a single space", in: "a\r\nb", want: "a b"},
		{name: "lone carriage return is dropped", in: "a\rb", want: "ab"},
		{name: "consecutive newlines", in: "a\n\nb", want: "a  b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, singleLineFieldValue(tt.in))
		})
	}
}
