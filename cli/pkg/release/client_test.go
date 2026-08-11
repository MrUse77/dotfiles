package release

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeHTTPDoer records requests and returns configured responses/errors.
type fakeHTTPDoer struct {
	requests []*http.Request
	resp     *http.Response
	err      error
}

func (f *fakeHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	if f.resp == nil {
		return nil, errors.New("fakeHTTPDoer: no response configured")
	}
	// Give each request its own body so multiple reads are independent.
	resp := *f.resp
	if f.resp.Body != nil {
		body, _ := io.ReadAll(f.resp.Body)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		f.resp.Body = io.NopCloser(bytes.NewReader(body))
	}
	return &resp, nil
}

func TestGitHubClient_Latest_Anonymous(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v1.2.3"}`)),
		},
	}
	client := NewGitHubClient(doer, "")

	release, err := client.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest error = %v", err)
	}
	if release.Tag != "v1.2.3" {
		t.Fatalf("Tag = %q, want v1.2.3", release.Tag)
	}
	if len(doer.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(doer.requests))
	}
	req := doer.requests[0]
	if req.Method != http.MethodGet {
		t.Fatalf("Method = %q, want GET", req.Method)
	}
	if req.URL.String() != "https://api.github.com/repos/MrUse77/dotfiles/releases/latest" {
		t.Fatalf("URL = %q, want GitHub latest endpoint", req.URL.String())
	}
	if req.Header.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("Accept header = %q, want application/vnd.github+json", req.Header.Get("Accept"))
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatalf("Authorization header present for anonymous request")
	}
}

func TestGitHubClient_Latest_Authenticated(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v1.2.3"}`)),
		},
	}
	client := NewGitHubClient(doer, "ghp_example")

	if _, err := client.Latest(context.Background()); err != nil {
		t.Fatalf("Latest error = %v", err)
	}
	req := doer.requests[0]
	auth := req.Header.Get("Authorization")
	if auth != "Bearer ghp_example" {
		t.Fatalf("Authorization header = %q, want Bearer ghp_example", auth)
	}
}

func TestGitHubClient_Latest_MissingTagName(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"name":"release"}`)),
		},
	}
	client := NewGitHubClient(doer, "")

	_, err := client.Latest(context.Background())
	if err == nil {
		t.Fatalf("expected error for missing tag_name")
	}
	var me *MalformedReleaseError
	if !errors.As(err, &me) {
		t.Fatalf("error type = %T, want *MalformedReleaseError", err)
	}
}

func TestGitHubClient_Latest_MalformedJSON(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{not json}`)),
		},
	}
	client := NewGitHubClient(doer, "")

	_, err := client.Latest(context.Background())
	if err == nil {
		t.Fatalf("expected error for malformed JSON")
	}
	var me *MalformedReleaseError
	if !errors.As(err, &me) {
		t.Fatalf("error type = %T, want *MalformedReleaseError", err)
	}
}

func TestGitHubClient_Latest_TransportError(t *testing.T) {
	doer := &fakeHTTPDoer{err: errors.New("dial failure")}
	client := NewGitHubClient(doer, "")

	_, err := client.Latest(context.Background())
	if err == nil {
		t.Fatalf("expected error for transport failure")
	}
	var te *TransportError
	if !errors.As(err, &te) {
		t.Fatalf("error type = %T, want *TransportError", err)
	}
}

func TestGitHubClient_Latest_RateLimit(t *testing.T) {
	statuses := []int{http.StatusForbidden, http.StatusTooManyRequests}
	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			doer := &fakeHTTPDoer{
				resp: &http.Response{
					StatusCode: status,
					Body:       io.NopCloser(strings.NewReader("rate limited")),
				},
			}
			client := NewGitHubClient(doer, "")

			_, err := client.Latest(context.Background())
			if err == nil {
				t.Fatalf("expected error for status %d", status)
			}
			var re *RateLimitError
			if !errors.As(err, &re) {
				t.Fatalf("error type = %T, want *RateLimitError", err)
			}
			if re.StatusCode != status {
				t.Fatalf("StatusCode = %d, want %d", re.StatusCode, status)
			}
		})
	}
}

func TestGitHubClient_Latest_OtherNon200(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Body:       io.NopCloser(strings.NewReader("server error")),
		},
	}
	client := NewGitHubClient(doer, "")

	_, err := client.Latest(context.Background())
	if err == nil {
		t.Fatalf("expected error for non-200")
	}
	var he *HTTPStatusError
	if !errors.As(err, &he) {
		t.Fatalf("error type = %T, want *HTTPStatusError", err)
	}
	if he.StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode = %d, want 500", he.StatusCode)
	}
}

func TestGitHubClient_Latest_RequestHasBoundedDeadline(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v1.0.0"}`)),
		},
	}
	client := NewGitHubClient(doer, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Latest(ctx); err != nil {
		t.Fatalf("Latest error = %v", err)
	}
	req := doer.requests[0]
	deadline, ok := req.Context().Deadline()
	if !ok {
		t.Fatalf("request context has no deadline")
	}
	want := time.Now().Add(30 * time.Second)
	if deadline.After(want) {
		t.Fatalf("deadline %v exceeds 30s cap", deadline)
	}
	if deadline.Before(time.Now().Add(4*time.Second)) || deadline.After(time.Now().Add(6*time.Second)) {
		t.Fatalf("deadline %v should be near the parent 5s deadline", deadline)
	}
}

func TestGitHubClient_Latest_ParentDeadlineCapsAt30Seconds(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v1.0.0"}`)),
		},
	}
	client := NewGitHubClient(doer, "")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	if _, err := client.Latest(ctx); err != nil {
		t.Fatalf("Latest error = %v", err)
	}
	req := doer.requests[0]
	deadline, ok := req.Context().Deadline()
	if !ok {
		t.Fatalf("request context has no deadline")
	}
	max := time.Now().Add(30 * time.Second)
	if deadline.After(max) {
		t.Fatalf("deadline %v exceeds 30s cap", deadline)
	}
}

func TestGitHubClient_Download_KeepsContextAliveForCaller(t *testing.T) {
	// Regression: the request context must stay alive after Download returns
	// (the caller reads the body afterwards) and must be canceled when the
	// caller closes the stream. A deferred cancel in Download previously
	// aborted real body reads with "context canceled".
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("binary data")),
		},
	}
	client := NewGitHubClient(doer, "")

	body, err := client.Download(context.Background(), Asset{Name: "asset", URL: "https://example.com/asset"})
	if err != nil {
		t.Fatalf("Download error = %v", err)
	}

	reqCtx := doer.requests[0].Context()
	if err := reqCtx.Err(); err != nil {
		t.Fatalf("request context canceled before caller read the body: %v", err)
	}

	if err := body.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if err := reqCtx.Err(); err == nil {
		t.Fatal("request context still alive after caller closed the stream")
	}
}

func TestGitHubClient_Download(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("binary data")),
		},
	}
	client := NewGitHubClient(doer, "")

	body, err := client.Download(context.Background(), Asset{Name: "asset", URL: "https://example.com/asset"})
	if err != nil {
		t.Fatalf("Download error = %v", err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll error = %v", err)
	}
	if string(data) != "binary data" {
		t.Fatalf("Download body = %q, want binary data", string(data))
	}
	req := doer.requests[0]
	if req.URL.String() != "https://example.com/asset" {
		t.Fatalf("Download URL = %q, want asset URL", req.URL.String())
	}
}

func TestGitHubClient_Download_PropagatesNon200(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Body:       io.NopCloser(strings.NewReader("missing")),
		},
	}
	client := NewGitHubClient(doer, "")

	_, err := client.Download(context.Background(), Asset{Name: "asset", URL: "https://example.com/asset"})
	if err == nil {
		t.Fatalf("expected error for 404")
	}
	var he *HTTPStatusError
	if !errors.As(err, &he) {
		t.Fatalf("error type = %T, want *HTTPStatusError", err)
	}
}

func TestBinaryAsset(t *testing.T) {
	release := Release{Assets: []Asset{
		{Name: "moonarch-cli-linux-amd64", URL: "u1"},
		{Name: "moonarch-cli-linux-arm64", URL: "u2"},
		{Name: "SHA256SUMS.txt", URL: "u3"},
	}}

	tests := []struct {
		name    string
		goarch  string
		want    string
		wantErr bool
	}{
		{name: "amd64", goarch: "amd64", want: "moonarch-cli-linux-amd64"},
		{name: "arm64", goarch: "arm64", want: "moonarch-cli-linux-arm64"},
		{name: "unsupported", goarch: "386", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset, err := BinaryAsset(release, tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				var ue *UnsupportedArchitectureError
				if !errors.As(err, &ue) {
					t.Fatalf("error type = %T, want *UnsupportedArchitectureError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if asset.Name != tt.want {
				t.Fatalf("asset.Name = %q, want %q", asset.Name, tt.want)
			}
		})
	}
}

func TestBinaryAsset_MissingAsset(t *testing.T) {
	release := Release{Assets: []Asset{{Name: "other", URL: "u"}}}
	_, err := BinaryAsset(release, "amd64")
	if err == nil {
		t.Fatalf("expected error for missing asset")
	}
	var ae *AssetNotFoundError
	if !errors.As(err, &ae) {
		t.Fatalf("error type = %T, want *AssetNotFoundError", err)
	}
}

func TestChecksumAsset(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		release := Release{Assets: []Asset{{Name: "SHA256SUMS.txt", URL: "u"}}}
		asset, err := ChecksumAsset(release)
		if err != nil {
			t.Fatalf("unexpected error = %v", err)
		}
		if asset.Name != "SHA256SUMS.txt" {
			t.Fatalf("asset.Name = %q", asset.Name)
		}
	})
	t.Run("missing", func(t *testing.T) {
		_, err := ChecksumAsset(Release{})
		if err == nil {
			t.Fatalf("expected error")
		}
		var ae *AssetNotFoundError
		if !errors.As(err, &ae) {
			t.Fatalf("error type = %T, want *AssetNotFoundError", err)
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		release := Release{Assets: []Asset{
			{Name: "SHA256SUMS.txt", URL: "u1"},
			{Name: "SHA256SUMS.txt", URL: "u2"},
		}}
		_, err := ChecksumAsset(release)
		if err == nil {
			t.Fatalf("expected error")
		}
		var ae *AssetNotFoundError
		if !errors.As(err, &ae) {
			t.Fatalf("error type = %T, want *AssetNotFoundError", err)
		}
	})
}

func TestGitHubClient_GetByTag(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"tag_name": "config-v1.2.3",
				"assets": [{"name": "config-v1.2.3.tar.zst", "browser_download_url": "https://example.com/artifact"}]
			}`)),
		},
	}
	client := NewGitHubClient(doer, "")

	release, err := client.GetByTag(context.Background(), "config-v1.2.3")
	if err != nil {
		t.Fatalf("GetByTag error = %v", err)
	}
	if release.Tag != "config-v1.2.3" {
		t.Fatalf("Tag = %q, want config-v1.2.3", release.Tag)
	}
	if len(release.Assets) != 1 || release.Assets[0].Name != "config-v1.2.3.tar.zst" {
		t.Fatalf("Assets = %+v", release.Assets)
	}
	if len(doer.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(doer.requests))
	}
	req := doer.requests[0]
	if req.Method != http.MethodGet {
		t.Fatalf("Method = %q, want GET", req.Method)
	}
	if req.URL.String() != "https://api.github.com/repos/MrUse77/dotfiles/releases/tags/config-v1.2.3" {
		t.Fatalf("URL = %q, want exact tags endpoint", req.URL.String())
	}
	if req.Header.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("Accept header = %q", req.Header.Get("Accept"))
	}
}

func TestGitHubClient_GetByTag_Authenticated(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"config-v1.2.3"}`)),
		},
	}
	client := NewGitHubClient(doer, "ghp_example")

	if _, err := client.GetByTag(context.Background(), "config-v1.2.3"); err != nil {
		t.Fatalf("GetByTag error = %v", err)
	}
	if auth := doer.requests[0].Header.Get("Authorization"); auth != "Bearer ghp_example" {
		t.Fatalf("Authorization header = %q, want Bearer ghp_example", auth)
	}
}

func TestGitHubClient_GetByTag_RejectsNonConfigSelectorBeforeRequest(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "latest", "config", "config-v1.2", "config-v1.2.3-beta"} {
		t.Run(tag, func(t *testing.T) {
			doer := &fakeHTTPDoer{}
			client := NewGitHubClient(doer, "")

			_, err := client.GetByTag(context.Background(), tag)
			if err == nil {
				t.Fatalf("expected rejection for %q", tag)
			}
			var ive *InvalidConfigVersionError
			if !errors.As(err, &ive) {
				t.Fatalf("error type = %T, want *InvalidConfigVersionError", err)
			}
			if len(doer.requests) != 0 {
				t.Fatalf("requests = %d, want 0 (selector must be rejected before any HTTP call)", len(doer.requests))
			}
		})
	}
}

func TestGitHubClient_GetByTag_MissingTagHasNoFallback(t *testing.T) {
	doer := &fakeHTTPDoer{
		resp: &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
		},
	}
	client := NewGitHubClient(doer, "")

	_, err := client.GetByTag(context.Background(), "config-v9.9.9")
	if err == nil {
		t.Fatalf("expected error for missing tag")
	}
	var he *HTTPStatusError
	if !errors.As(err, &he) {
		t.Fatalf("error type = %T, want *HTTPStatusError", err)
	}
	if he.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want 404", he.StatusCode)
	}
	if len(doer.requests) != 1 {
		t.Fatalf("requests = %d, want exactly 1 (no fallback retry)", len(doer.requests))
	}
}
