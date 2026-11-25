package list

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_listOrgs(t *testing.T) {
	type args struct {
		limit int
	}

	tests := []struct {
		name     string
		args     args
		httpStub func(*testing.T, *httpmock.Registry)
		wantErr  bool
	}{
		{
			name: "default",
			args: args{
				limit: 30,
			},
			httpStub: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/user"),
					httpmock.StringResponse(`{"login":"octocat"}`))
				reg.Register(
					httpmock.QueryMatcher("GET", "api/v3/user/orgs", map[string][]string{"per_page": {"30"}}),
					httpmock.StringResponse(`[]`))
			},
		},
		{
			name: "with limit",
			args: args{
				limit: 1,
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "api/v3/user"),
					httpmock.StringResponse(`{"login":"octocat"}`))
				r.Register(
					httpmock.QueryMatcher("GET", "api/v3/user/orgs", map[string][]string{"per_page": {"1"}}),
					httpmock.StringResponse(`[{"login":"github"}]`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStub != nil {
				tt.httpStub(t, reg)
			}
			httpClient := &http.Client{Transport: reg}

			_, err := listOrgs(httpClient, "octocat", tt.args.limit)
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}
