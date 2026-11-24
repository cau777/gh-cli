package list

import (
	"net/http"
	"net/url"
	"testing"

	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/require"
)

func Test_ListPullRequests(t *testing.T) {
	type args struct {
		detector fd.Detector
		repo     ghrepo.Interface
		filters  prShared.FilterOptions
		limit    int
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
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
				filters: prShared.FilterOptions{
					State: "open",
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/pulls", url.Values{
						"per_page": {"30"},
						"state":    {"open"},
					}),
					httpmock.StringResponse(`[]`))
			},
		},
		{
			name: "closed",
			args: args{
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
				filters: prShared.FilterOptions{
					State: "closed",
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/pulls", url.Values{
						"per_page": {"30"},
						"state":    {"closed"},
					}),
					httpmock.StringResponse(`[]`))
			},
		},
		{
			name: "with labels",
			args: args{
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
				filters: prShared.FilterOptions{
					State:  "open",
					Labels: []string{"hello", "one world"},
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.QueryMatcher("GET", "search/issues", url.Values{
						"per_page": {"30"},
						"q":        {`label:"one world" label:hello repo:OWNER/REPO state:open type:pr`},
					}),
					httpmock.StringResponse(`{"total_count": 0, "items": []}`))
			},
		},
		{
			name: "with author",
			args: args{
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
				filters: prShared.FilterOptions{
					State:  "open",
					Author: "monalisa",
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.QueryMatcher("GET", "search/issues", url.Values{
						"per_page": {"30"},
						"q":        {"author:monalisa repo:OWNER/REPO state:open type:pr"},
					}),
					httpmock.StringResponse(`{"total_count": 0, "items": []}`))
			},
		},
		{
			name: "with search",
			args: args{
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
				filters: prShared.FilterOptions{
					State:  "open",
					Search: "one world in:title",
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.QueryMatcher("GET", "search/issues", url.Values{
						"per_page": {"30"},
						"q":        {"( one world in:title ) repo:OWNER/REPO state:open type:pr"},
					}),
					httpmock.StringResponse(`{"total_count": 0, "items": []}`))
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

			_, err := listPullRequests(httpClient, tt.args.detector, tt.args.repo, tt.args.filters, tt.args.limit)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
