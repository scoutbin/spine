package release_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/scoutbin/spine/internal/release"
)

func TestLatestTag(t *testing.T) {
	testcases := []struct {
		name       string
		token      string
		noTool     bool
		status     int
		body       string
		wantTag    string
		wantErr    string
		wantAuth   string
		trailingSl bool
	}{
		{name: "latest tag", status: 200, body: `{"tag_name":"v0.2.0","name":"x"}`, wantTag: "v0.2.0"},
		{name: "token sent as bearer", token: "s3cret", status: 200, body: `{"tag_name":"v0.2.0"}`, wantTag: "v0.2.0", wantAuth: "Bearer s3cret"},
		{name: "base URL trailing slash", status: 200, body: `{"tag_name":"v0.2.0"}`, wantTag: "v0.2.0", trailingSl: true},
		{name: "404 means no release", status: 404, body: `{"message":"Not Found"}`, wantErr: "no release found"},
		{name: "403 rate limit", status: 403, body: `{"message":"API rate limit exceeded"}`, wantErr: "rate limit"},
		{name: "429 rate limit", status: 429, wantErr: "rate limit"},
		{name: "500", status: 500, wantErr: "HTTP 500"},
		{name: "503", status: 503, wantErr: "HTTP 503"},
		{name: "malformed JSON", status: 200, body: `{"tag_name":`, wantErr: "decode GitHub response"},
		{name: "missing tag_name", status: 200, body: `{}`, wantErr: "no tag_name"},
		{name: "non-semver tag", status: 200, body: `{"tag_name":"nightly"}`, wantErr: `invalid tag "nightly"`},
		{name: "tool not set", noTool: true, wantErr: "release.Client: Tool is not set"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotAuth, gotAccept, gotUA string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
				gotAccept, gotUA = r.Header.Get("Accept"), r.Header.Get("User-Agent")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			base := srv.URL
			if tc.trailingSl {
				base += "/"
			}

			client := release.Client{Tool: tool, BaseURL: base, HTTP: srv.Client(), Token: tc.token}
			if tc.noTool {
				client.Tool.Repo = ""
			}

			tag, err := client.LatestTag(context.Background())

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LatestTag: %v", err)
			}
			if tag != tc.wantTag {
				t.Errorf("tag = %q, want %q", tag, tc.wantTag)
			}
			if gotPath != "/repos/scoutbin/demo/releases/latest" {
				t.Errorf("path = %q, want %q", gotPath, "/repos/scoutbin/demo/releases/latest")
			}
			if gotAuth != tc.wantAuth {
				t.Errorf("Authorization = %q, want %q", gotAuth, tc.wantAuth)
			}
			if gotAccept == "" || gotUA != tool.Name {
				t.Errorf("Accept = %q, User-Agent = %q, want Accept set and User-Agent %q", gotAccept, gotUA, tool.Name)
			}
		})
	}
}

func TestReleaseTags(t *testing.T) {
	const bodyCap = 1 << 20
	atCap := `[{"tag_name":"v0.2.0"}]` + strings.Repeat(" ", bodyCap-len(`[{"tag_name":"v0.2.0"}]`))

	overCap := `[{"tag_name":"v0.2.0"},` + strings.Repeat(" ", bodyCap) + `{"tag_name":"v0.1.0"}]`

	testcases := []struct {
		name     string
		status   int
		body     string
		wantTags []string
		wantErr  string
	}{
		{name: "all published tags", status: 200, body: `[{"tag_name":"v0.2.0"},{"tag_name":"v0.3.0-rc.1","prerelease":true}]`, wantTags: []string{"v0.2.0", "v0.3.0-rc.1"}},
		{name: "drafts dropped", status: 200, body: `[{"tag_name":"v0.2.0","draft":true},{"tag_name":"v0.1.0","draft":false}]`, wantTags: []string{"v0.1.0"}},
		{name: "unparseable tags ignored", status: 200, body: `[{"tag_name":"nightly"},{"tag_name":""},{"tag_name":"v0.1.0"},{"tag_name":"1.2"}]`, wantTags: []string{"v0.1.0"}},
		{name: "empty list", status: 200, body: `[]`, wantTags: []string{}},
		{name: "body at the size cap", status: 200, body: atCap, wantTags: []string{"v0.2.0"}},
		{name: "body over the size cap", status: 200, body: overCap, wantErr: "decode GitHub response"},
		{name: "malformed JSON", status: 200, body: `[{"tag_name":`, wantErr: "decode GitHub response"},
		{name: "not a list", status: 200, body: `{"tag_name":"v0.2.0"}`, wantErr: "decode GitHub response"},
		{name: "404", status: 404, wantErr: "no release found"},
		{name: "403 rate limit", status: 403, wantErr: "rate limit"},
		{name: "429 rate limit", status: 429, wantErr: "rate limit"},
		{name: "500", status: 500, wantErr: "HTTP 500"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotQuery, gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery, gotAuth = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			tags, err := release.Client{Tool: tool, BaseURL: srv.URL, HTTP: srv.Client(), Token: "s3cret"}.ReleaseTags(context.Background())

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReleaseTags: %v", err)
			}
			if !slices.Equal(tags, tc.wantTags) {
				t.Errorf("tags = %q, want %q", tags, tc.wantTags)
			}
			if gotPath != "/repos/scoutbin/demo/releases" || gotQuery != "per_page=30" {
				t.Errorf("request = %q?%q, want /repos/scoutbin/demo/releases?per_page=30", gotPath, gotQuery)
			}
			if gotAuth != "Bearer s3cret" {
				t.Errorf("Authorization = %q, want the bearer token", gotAuth)
			}
		})
	}
}

func TestLatestTagHonorsContext(t *testing.T) {
	unblock := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-unblock }))
	defer srv.Close()
	defer close(unblock)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err := (release.Client{Tool: tool, BaseURL: srv.URL, HTTP: srv.Client()}).LatestTag(ctx); err == nil {
		t.Fatal("err = nil, want a context error")
	}
}

func TestOpenAsset(t *testing.T) {
	asset := tool.ArchiveName("v0.2.0", "linux", "amd64")
	testcases := []struct {
		name          string
		status        int
		body          string
		trailingSlash bool
		wantErr       string
	}{
		{name: "streams the asset", status: 200, body: "payload"},
		{name: "base URL trailing slash", status: 200, body: "payload", trailingSlash: true},
		{name: "404 names the asset and tag", status: 404, wantErr: asset + " not found in release v0.2.0"},
		{name: "500", status: 500, wantErr: "HTTP 500"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotAuth, gotUA string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotAuth, gotUA = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("User-Agent")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			base := srv.URL
			if tc.trailingSlash {
				base += "/"
			}

			body, size, err := release.Client{Tool: tool, DownloadBase: base, HTTP: srv.Client(), Token: "s3cret"}.OpenAsset(context.Background(), "v0.2.0", asset)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenAsset: %v", err)
			}
			defer body.Close()
			got, _ := io.ReadAll(body)
			if string(got) != tc.body || size != int64(len(tc.body)) {
				t.Errorf("body = %q size = %d, want %q size %d", got, size, tc.body, len(tc.body))
			}
			if gotPath != "/v0.2.0/"+asset {
				t.Errorf("path = %q, want /v0.2.0/%s", gotPath, asset)
			}
			if gotUA != tool.Name {
				t.Errorf("User-Agent = %q, want %q", gotUA, tool.Name)
			}
			if gotAuth != "" {
				t.Errorf("Authorization = %q, want none for asset downloads", gotAuth)
			}
		})
	}
}
