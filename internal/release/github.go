package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/scoutbin/spine/internal/manifest"
)

const (
	defaultAPIBase = "https://api.github.com"
	lookupTimeout  = 15 * time.Second
	maxBodyBytes   = 1 << 20
)

var errToolNotSet = errors.New("release.Client: Tool is not set")

// Client talks to the GitHub REST API about the releases of Tool, which must be
// set. By default it targets api.github.com with http.DefaultClient and no
// authentication.
type Client struct {
	Tool manifest.Tool

	BaseURL string       // defaults to https://api.github.com
	HTTP    *http.Client // defaults to http.DefaultClient
	Token   string       // sent as a bearer token when non-empty

	DownloadBase string // defaults to the GitHub release download URL
}

// LatestTag returns the tag of the latest published release (GitHub excludes
// drafts and prereleases). The lookup is bounded by a timeout and ctx.
func (c Client) LatestTag(ctx context.Context) (string, error) {
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := c.getJSON(ctx, "/repos/"+c.Tool.Repo+"/releases/latest", &body); err != nil {
		return "", err
	}
	if body.TagName == "" {
		return "", errors.New("GitHub response has no tag_name")
	}
	if _, ok := ParseVersion(body.TagName); !ok {
		return "", fmt.Errorf("latest release has invalid tag %q: want a semantic version such as v0.2.0", body.TagName)
	}
	return body.TagName, nil
}

// ReleaseTags returns the tags of the newest published releases, prereleases
// included (the first page of the releases list). Drafts and tags that are not
// semantic versions are dropped. The list order is not documented, so callers
// choose by version, never by position.
func (c Client) ReleaseTags(ctx context.Context) ([]string, error) {
	var body []struct {
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
	}
	if err := c.getJSON(ctx, "/repos/"+c.Tool.Repo+"/releases?per_page=30", &body); err != nil {
		return nil, err
	}
	tags := []string{}
	for _, r := range body {
		if _, ok := ParseVersion(r.TagName); ok && !r.Draft {
			tags = append(tags, r.TagName)
		}
	}
	return tags, nil
}

// getJSON GETs path from the API and decodes the response into out.
func (c Client) getJSON(ctx context.Context, path string, out any) error {
	if c.Tool.Repo == "" {
		return errToolNotSet
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	base := c.BaseURL
	if base == "" {
		base = defaultAPIBase
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.Tool.Name)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return errors.New("no release found")
	case http.StatusForbidden, http.StatusTooManyRequests:
		return fmt.Errorf("GitHub refused the request (HTTP %d), usually the API rate limit; set GITHUB_TOKEN to raise it", resp.StatusCode)
	default:
		return fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

// OpenAsset starts downloading a release asset and returns its body and size
// (-1 when unknown). The caller closes the body, and ctx bounds the whole read.
func (c Client) OpenAsset(ctx context.Context, tag, asset string) (io.ReadCloser, int64, error) {
	if c.Tool.Repo == "" {
		return nil, 0, errToolNotSet
	}
	url := DownloadURL(c.Tool, tag, asset)
	if c.DownloadBase != "" {
		url = strings.TrimSuffix(c.DownloadBase, "/") + "/" + tag + "/" + asset
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.Tool.Name)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Body, resp.ContentLength, nil
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, 0, fmt.Errorf("%s not found in release %s", asset, tag)
	default:
		resp.Body.Close()
		return nil, 0, fmt.Errorf("download %s: HTTP %d", asset, resp.StatusCode)
	}
}
