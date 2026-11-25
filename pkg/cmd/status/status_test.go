package status

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

type testHostConfig string

func (c testHostConfig) DefaultHost() (string, string) {
	return string(c), ""
}

func TestNewCmdStatus(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants StatusOptions
	}{
		{
			name: "defaults",
		},
		{
			name: "org",
			cli:  "-o cli",
			wants: StatusOptions{
				Org: "cli",
			},
		},
		{
			name: "exclude",
			cli:  "-e cli/cli,cli/go-gh",
			wants: StatusOptions{
				Exclude: []string{"cli/cli", "cli/go-gh"},
			},
		},
	}

	for _, tt := range tests {
		ios, _, _, _ := iostreams.Test()
		ios.SetStdinTTY(true)
		ios.SetStdoutTTY(true)

		if tt.wants.Exclude == nil {
			tt.wants.Exclude = []string{}
		}

		f := &cmdutil.Factory{
			IOStreams: ios,
			Config: func() (gh.Config, error) {
				return config.NewMockConfig(), nil
			},
		}
		t.Run(tt.name, func(t *testing.T) {
			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *StatusOptions
			cmd := NewCmdStatus(f, func(opts *StatusOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Org, gotOpts.Org)
			assert.Equal(t, tt.wants.Exclude, gotOpts.Exclude)
		})
	}
}

func testSearchMatcher(prefix string) httpmock.Matcher {
	return func(req *http.Request) bool {
		return strings.EqualFold(req.Method, http.MethodGet) &&
			strings.HasSuffix(req.URL.Path, "/search/issues") &&
			strings.HasPrefix(req.URL.Query().Get("q"), prefix)
	}
}

func stubEmptySearch(reg *httpmock.Registry) {
	reg.Register(
		testSearchMatcher("assignee:@me"),
		httpmock.StringResponse(`{"items": []}`),
	)
	reg.Register(
		testSearchMatcher("review-requested:@me"),
		httpmock.StringResponse(`{"items": []}`),
	)
}

func stubFixtureSearch(reg *httpmock.Registry) {
	reg.Register(
		testSearchMatcher("assignee:@me"),
		httpmock.FileResponse("./fixtures/search_assignments.json"),
	)
	reg.Register(
		testSearchMatcher("review-requested:@me"),
		httpmock.FileResponse("./fixtures/search_review_requested.json"),
	)
}

func TestStatusRun(t *testing.T) {
	tests := []struct {
		name       string
		httpStubs  func(*httpmock.Registry)
		opts       *StatusOptions
		wantOut    string
		wantErrMsg string
	}{
		{
			name: "nothing",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				stubEmptySearch(reg)
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.StringResponse(`[]`))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.StringResponse(`[]`))
			},
			opts:    &StatusOptions{},
			wantOut: "Assigned Issues                       │ Assigned Pull Requests                \nNothing here ^_^                      │ Nothing here ^_^                      \n                                      │                                       \nReview Requests                       │ Mentions                              \nNothing here ^_^                      │ Nothing here ^_^                      \n                                      │                                       \nRepository Activity\nNothing here ^_^\n\n",
		},
		{
			name: "notifications 404 is tolerated",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				stubEmptySearch(reg)
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.StringResponse(`[]`))
			},
			opts:    &StatusOptions{},
			wantOut: "Assigned Issues                       │ Assigned Pull Requests                \nNothing here ^_^                      │ Nothing here ^_^                      \n                                      │                                       \nReview Requests                       │ Mentions                              \nNothing here ^_^                      │ Nothing here ^_^                      \n                                      │                                       \nRepository Activity\nNothing here ^_^\n\n",
		},
		{
			name: "something",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/110"),
					httpmock.StringResponse(`{"body":"hello @jillvalentine how are you"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/4113"),
					httpmock.StringResponse(`{"body":"this is a comment"}`))
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/issues/1096"),
					httpmock.StringResponse(`{"body":"@jillvalentine hi"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/comments/1065"),
					httpmock.StringResponse(`{"body":"not a real mention"}`))
				reg.Register(
					httpmock.REST("GET", "repos/vilmibm/gh-screensaver/issues/comments/10"),
					httpmock.StringResponse(`{"body":"a message for @jillvalentine"}`))
				stubFixtureSearch(reg)
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.FileResponse("./fixtures/notifications.json"))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.FileResponse("./fixtures/events.json"))
			},
			opts:    &StatusOptions{},
			wantOut: "Assigned Issues                       │ Assigned Pull Requests                \nvilmibm/testing#157     yolo          │ cli/cli#5272  Pin extensions          \ncli/cli#3223            Repo garden...│ rpd/todo#73   Board up RPD windows    \nrpd/todo#514            Reducing zo...│ cli/cli#4768  Issue Frecency          \nvilmibm/testing#74      welp          │                                       \nadreyer/arkestrator#22  complete mo...│                                       \n                                      │                                       \nReview Requests                       │ Mentions                              \ncli/cli#5272          Pin extensions  │ rpd/todo#110               hello @j...\nvilmibm/testing#1234  Foobar          │ cli/cli#1096               @jillval...\nrpd/todo#50           Welcome party...│ vilmibm/gh-screensaver#15  a messag...\ncli/cli#4671          This pull req...│                                       \nrpd/todo#49           Haircut for Leon│                                       \n                                      │                                       \nRepository Activity\nrpd/todo#5326         new PR                        Only write UTF-8 BOM on W...\nvilmibm/testing#5325  comment on Ability to sea...  We are working on dedicat...\ncli/cli#5319          comment on [Codespaces] D...  Wondering if we shouldn't...\ncli/cli#5300          new issue                     Terminal bell when a runn...\n\n",
		},
		{
			name: "exclude a repository",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/110"),
					httpmock.StringResponse(`{"body":"hello @jillvalentine how are you"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/4113"),
					httpmock.StringResponse(`{"body":"this is a comment"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/comments/1065"),
					httpmock.StringResponse(`{"body":"not a real mention"}`))
				reg.Register(
					httpmock.REST("GET", "repos/vilmibm/gh-screensaver/issues/comments/10"),
					httpmock.StringResponse(`{"body":"a message for @jillvalentine"}`))
				stubFixtureSearch(reg)
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.FileResponse("./fixtures/notifications.json"))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.FileResponse("./fixtures/events.json"))
			},
			opts: &StatusOptions{
				Exclude: []string{"cli/cli"},
			},
			// NOTA BENE: you'll see cli/cli in search results because that happens
			// server side and the fixture doesn't account for that
			wantOut: "Assigned Issues                       │ Assigned Pull Requests                \nvilmibm/testing#157     yolo          │ cli/cli#5272  Pin extensions          \ncli/cli#3223            Repo garden...│ rpd/todo#73   Board up RPD windows    \nrpd/todo#514            Reducing zo...│ cli/cli#4768  Issue Frecency          \nvilmibm/testing#74      welp          │                                       \nadreyer/arkestrator#22  complete mo...│                                       \n                                      │                                       \nReview Requests                       │ Mentions                              \ncli/cli#5272          Pin extensions  │ rpd/todo#110               hello @j...\nvilmibm/testing#1234  Foobar          │ vilmibm/gh-screensaver#15  a messag...\nrpd/todo#50           Welcome party...│                                       \ncli/cli#4671          This pull req...│                                       \nrpd/todo#49           Haircut for Leon│                                       \n                                      │                                       \nRepository Activity\nrpd/todo#5326         new PR                        Only write UTF-8 BOM on W...\nvilmibm/testing#5325  comment on Ability to sea...  We are working on dedicat...\n\n",
		},
		{
			name: "exclude repositories",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				reg.Register(
					httpmock.REST("GET", "repos/vilmibm/gh-screensaver/issues/comments/10"),
					httpmock.StringResponse(`{"body":"a message for @jillvalentine"}`))
				stubFixtureSearch(reg)
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.FileResponse("./fixtures/notifications.json"))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.FileResponse("./fixtures/events.json"))
			},
			opts: &StatusOptions{
				Exclude: []string{"cli/cli", "rpd/todo"},
			},
			// NOTA BENE: you'll see cli/cli in search results because that happens
			// server side and the fixture doesn't account for that
			wantOut: "Assigned Issues                       │ Assigned Pull Requests                \nvilmibm/testing#157     yolo          │ cli/cli#5272  Pin extensions          \ncli/cli#3223            Repo garden...│ rpd/todo#73   Board up RPD windows    \nrpd/todo#514            Reducing zo...│ cli/cli#4768  Issue Frecency          \nvilmibm/testing#74      welp          │                                       \nadreyer/arkestrator#22  complete mo...│                                       \n                                      │                                       \nReview Requests                       │ Mentions                              \ncli/cli#5272          Pin extensions  │ vilmibm/gh-screensaver#15  a messag...\nvilmibm/testing#1234  Foobar          │                                       \nrpd/todo#50           Welcome party...│                                       \ncli/cli#4671          This pull req...│                                       \nrpd/todo#49           Haircut for Leon│                                       \n                                      │                                       \nRepository Activity\nvilmibm/testing#5325  comment on Ability to sea...  We are working on dedicat...\n\n",
		},
		{
			name: "filter to an org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/110"),
					httpmock.StringResponse(`{"body":"hello @jillvalentine how are you"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/4113"),
					httpmock.StringResponse(`{"body":"this is a comment"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/comments/1065"),
					httpmock.StringResponse(`{"body":"not a real mention"}`))
				stubFixtureSearch(reg)
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.FileResponse("./fixtures/notifications.json"))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.FileResponse("./fixtures/events.json"))
			},
			opts: &StatusOptions{
				Org: "rpd",
			},
			wantOut: "Assigned Issues                       │ Assigned Pull Requests                \nvilmibm/testing#157     yolo          │ cli/cli#5272  Pin extensions          \ncli/cli#3223            Repo garden...│ rpd/todo#73   Board up RPD windows    \nrpd/todo#514            Reducing zo...│ cli/cli#4768  Issue Frecency          \nvilmibm/testing#74      welp          │                                       \nadreyer/arkestrator#22  complete mo...│                                       \n                                      │                                       \nReview Requests                       │ Mentions                              \ncli/cli#5272          Pin extensions  │ rpd/todo#110  hello @jillvalentine ...\nvilmibm/testing#1234  Foobar          │                                       \nrpd/todo#50           Welcome party...│                                       \ncli/cli#4671          This pull req...│                                       \nrpd/todo#49           Haircut for Leon│                                       \n                                      │                                       \nRepository Activity\nrpd/todo#5326  new PR  Only write UTF-8 BOM on Windows where it is needed\n\n",
		},
		{
			name: "forbidden errors",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/110"),
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							Request:    req,
							StatusCode: 403,
							Body:       io.NopCloser(strings.NewReader(`{"message": "You don't have permission to access this resource."}`)),
							Header: http.Header{
								"Content-Type": []string{"application/json"},
								"X-Github-Sso": []string{"required; url=http://click.me/auth"},
							},
						}, nil
					})
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/4113"),
					httpmock.StringResponse(`{"body":"this is a comment"}`))
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/issues/1096"),
					httpmock.StringResponse(`{"body":"@jillvalentine hi"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/comments/1065"),
					httpmock.StringResponse(`{"body":"not a real mention"}`))
				reg.Register(
					httpmock.REST("GET", "repos/vilmibm/gh-screensaver/issues/comments/10"),
					httpmock.StringResponse(`{"body":"a message for @jillvalentine"}`))
				reg.Register(
					testSearchMatcher("assignee:@me"),
					httpmock.StringResponse(`{
						"items": [
							{
								"updated_at": "2022-03-15T17:10:25Z",
								"pull_request": {},
								"title": "Pin extensions",
								"number": 5272,
								"repository_url": "https://api.github.com/repos/cli/cli"
							},
							{
								"updated_at": "2022-03-10T21:33:57Z",
								"title": "yolo",
								"number": 157,
								"repository_url": "https://api.github.com/repos/vilmibm/testing"
							}
						]
					}`))
				reg.Register(
					testSearchMatcher("review-requested:@me"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(403, `{"message":"Resource protected by organization SAML enforcement. You must grant your OAuth token access to this organization."}`),
						"Content-Type",
						"application/json",
					))
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.FileResponse("./fixtures/notifications.json"))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.FileResponse("./fixtures/events.json"))
			},
			opts: &StatusOptions{},
			wantOut: heredoc.Doc(`
				Assigned Issues                       │ Assigned Pull Requests                
				vilmibm/testing#157  yolo             │ cli/cli#5272  Pin extensions          
				                                      │                                       
				Review Requests                       │ Mentions                              
				Nothing here ^_^                      │ cli/cli#1096               @jillval...
				                                      │ vilmibm/gh-screensaver#15  a messag...
				                                                                              
				Repository Activity
				rpd/todo#5326         new PR                        Only write UTF-8 BOM on W...
				vilmibm/testing#5325  comment on Ability to sea...  We are working on dedicat...
				cli/cli#5319          comment on [Codespaces] D...  Wondering if we shouldn't...
				cli/cli#5300          new issue                     Terminal bell when a runn...
				
				warning: Resource protected by organization SAML enforcement. You must grant your OAuth token access to this organization.
				warning: You don't have permission to access this resource.
			`),
		},
		{
			name: "not found errors",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/110"),
					httpmock.StringResponse(`{"body":"hello @jillvalentine how are you"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/4113"),
					httpmock.StringResponse(`{"body":"this is a comment"}`))
				reg.Register(
					httpmock.REST("GET", "repos/cli/cli/issues/1096"),
					httpmock.StringResponse(`{"body":"@jillvalentine hi"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/comments/1065"),
					httpmock.StatusStringResponse(404, `{
						"message": "Not Found."
					}`))
				reg.Register(
					httpmock.REST("GET", "repos/vilmibm/gh-screensaver/issues/comments/10"),
					httpmock.StringResponse(`{"body":"a message for @jillvalentine"}`))
				stubFixtureSearch(reg)
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.FileResponse("./fixtures/notifications.json"))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.FileResponse("./fixtures/events.json"))
			},
			opts: &StatusOptions{},
			wantOut: heredoc.Doc(`
				Assigned Issues                       │ Assigned Pull Requests                
				vilmibm/testing#157     yolo          │ cli/cli#5272  Pin extensions          
				cli/cli#3223            Repo garden...│ rpd/todo#73   Board up RPD windows    
				rpd/todo#514            Reducing zo...│ cli/cli#4768  Issue Frecency          
				vilmibm/testing#74      welp          │                                       
				adreyer/arkestrator#22  complete mo...│                                       
				                                      │                                       
				Review Requests                       │ Mentions                              
				cli/cli#5272          Pin extensions  │ rpd/todo#110               hello @j...
				vilmibm/testing#1234  Foobar          │ cli/cli#1096               @jillval...
				rpd/todo#50           Welcome party...│ vilmibm/gh-screensaver#15  a messag...
				cli/cli#4671          This pull req...│                                       
				rpd/todo#49           Haircut for Leon│                                       
				                                      │                                       
				Repository Activity
				rpd/todo#5326         new PR                        Only write UTF-8 BOM on W...
				vilmibm/testing#5325  comment on Ability to sea...  We are working on dedicat...
				cli/cli#5319          comment on [Codespaces] D...  Wondering if we shouldn't...
				cli/cli#5300          new issue                     Terminal bell when a runn...
				
			`),
		},
		{
			name: "notification errors",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				reg.Register(
					httpmock.REST("GET", "repos/rpd/todo/issues/110"),
					httpmock.StatusStringResponse(429, `{
						"message": "Too many requests."
					}`))
				stubEmptySearch(reg)
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.StringResponse(`[
						{
							"reason": "mention",
							"subject": {
								"title": "Good",
								"url": "https://api.github.com/repos/rpd/todo/issues/110",
								"latest_comment_url": "https://api.github.com/repos/rpd/todo/issues/110",
								"type": "Issue"
							},
							"repository": {
								"full_name": "rpd/todo",
								"owner": {
									"login": "rpd"
								}
							}
						}
					]`))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.StringResponse(`[]`))
			},
			opts:       &StatusOptions{},
			wantErrMsg: "could not load notifications: could not fetch comment: HTTP 429 (https://api.github.com/repos/rpd/todo/issues/110)",
		},
		{
			name: "search errors",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "user"),
					httpmock.StringResponse(`{"login":"jillvalentine"}`))
				reg.Register(
					testSearchMatcher("assignee:@me"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
						"Content-Type",
						"application/json",
					))
				reg.Register(
					testSearchMatcher("review-requested:@me"),
					httpmock.StringResponse(`{"items": []}`))
				reg.Register(
					httpmock.REST("GET", "notifications"),
					httpmock.StringResponse(`[]`))
				reg.Register(
					httpmock.REST("GET", "users/jillvalentine/received_events"),
					httpmock.StringResponse(`[]`))
			},
			opts:       &StatusOptions{},
			wantErrMsg: "failed to search: could not search for assignments: HTTP 404: Not Found (https://api.github.com/search/issues?per_page=25&q=assignee%3A%40me+state%3Aopen+archived%3Afalse)",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		tt.opts.CachedClient = func(c *http.Client, _ time.Duration) *http.Client {
			return c
		}
		tt.opts.HostConfig = testHostConfig("github.com")
		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(true)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			err := statusRun(tt.opts)
			if tt.wantErrMsg != "" {
				assert.Equal(t, tt.wantErrMsg, err.Error())
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
