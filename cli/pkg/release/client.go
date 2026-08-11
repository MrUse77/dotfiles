package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTPDoer is the injectable HTTP client boundary.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Asset represents a GitHub release asset.
type Asset struct {
	Name string
	URL  string
}

// Release is the normalized GitHub latest release metadata.
type Release struct {
	Tag    string
	Assets []Asset
}

// releaseResponse is the GitHub API JSON shape we decode.
type releaseResponse struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Client is the release client interface used by the update orchestrator.
type Client interface {
	Latest(ctx context.Context) (Release, error)               // CLI self-update only
	GetByTag(ctx context.Context, tag string) (Release, error) // config apply/rollback; rejects non-config-v* and never falls back
	Download(ctx context.Context, asset Asset) (io.ReadCloser, error)
}

// GitHubClient queries the GitHub releases API without shell-outs or caches.
type GitHubClient struct {
	doer  HTTPDoer
	token string
}

// NewGitHubClient creates a release client with the supplied HTTP boundary and
// optional bearer token.
func NewGitHubClient(doer HTTPDoer, token string) *GitHubClient {
	return &GitHubClient{doer: doer, token: token}
}

const (
	latestEndpoint  = "https://api.github.com/repos/MrUse77/dotfiles/releases/latest"
	tagsEndpointFmt = "https://api.github.com/repos/MrUse77/dotfiles/releases/tags/%s"
	requestTimeout  = 30 * time.Second
)

// Latest fetches the latest release metadata from GitHub.
func (c *GitHubClient) Latest(ctx context.Context) (Release, error) {
	return c.fetchRelease(ctx, latestEndpoint)
}

// GetByTag fetches the release metadata for the exact tag. It rejects any
// selector that is not an exact config-vMAJOR.MINOR.PATCH tag before making a
// request, and it never falls back to another release when the tag is missing.
func (c *GitHubClient) GetByTag(ctx context.Context, tag string) (Release, error) {
	if _, err := ParseConfigVersion(tag); err != nil {
		return Release{}, err
	}
	return c.fetchRelease(ctx, fmt.Sprintf(tagsEndpointFmt, url.PathEscape(tag)))
}

// fetchRelease performs the shared GitHub release metadata request and decode.
func (c *GitHubClient) fetchRelease(ctx context.Context, endpoint string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, &TransportError{Cause: err}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	reqCtx, cancel := cappedContext(ctx, requestTimeout)
	defer cancel()
	req = req.WithContext(reqCtx)

	resp, err := c.doer.Do(req)
	if err != nil {
		return Release{}, &TransportError{Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, classifyResponse(resp)
	}

	var raw releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Release{}, &MalformedReleaseError{Cause: err}
	}
	if raw.TagName == "" {
		return Release{}, &MalformedReleaseError{Cause: fmt.Errorf("missing tag_name")}
	}

	release := Release{Tag: raw.TagName}
	for _, a := range raw.Assets {
		release.Assets = append(release.Assets, Asset{Name: a.Name, URL: a.URL})
	}
	return release, nil
}

// Download opens a read stream for the requested asset. The returned reader
// must be closed by the caller: closing it also releases the request timeout.
// The cancel function is handed to the caller (via the wrapper) instead of
// being deferred here, so the response body remains readable after Download
// returns.
func (c *GitHubClient) Download(ctx context.Context, asset Asset) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return nil, &TransportError{Cause: err}
	}

	reqCtx, cancel := cappedContext(ctx, requestTimeout)
	req = req.WithContext(reqCtx)

	resp, err := c.doer.Do(req)
	if err != nil {
		cancel()
		return nil, &TransportError{Cause: err}
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		resp.Body.Close()
		return nil, classifyResponse(resp)
	}
	return &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}, nil
}

// cancelReadCloser cancels the request context when the caller closes the
// stream, keeping the bounded timeout alive for the whole body read.
type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func cappedContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(time.Now().Add(timeout)) {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// BinaryAsset selects the architecture-specific binary asset.
func BinaryAsset(release Release, goarch string) (Asset, error) {
	var name string
	switch goarch {
	case "amd64":
		name = "moonarch-cli-linux-amd64"
	case "arm64":
		name = "moonarch-cli-linux-arm64"
	default:
		return Asset{}, &UnsupportedArchitectureError{GOARCH: goarch}
	}
	for _, a := range release.Assets {
		if a.Name == name {
			return a, nil
		}
	}
	return Asset{}, &AssetNotFoundError{AssetName: name}
}

// ChecksumAsset selects the SHA256SUMS.txt asset.
func ChecksumAsset(release Release) (Asset, error) {
	var found *Asset
	for i := range release.Assets {
		if release.Assets[i].Name == "SHA256SUMS.txt" {
			if found != nil {
				return Asset{}, &AssetNotFoundError{AssetName: "SHA256SUMS.txt"}
			}
			found = &release.Assets[i]
		}
	}
	if found == nil {
		return Asset{}, &AssetNotFoundError{AssetName: "SHA256SUMS.txt"}
	}
	return *found, nil
}
