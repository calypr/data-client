package download

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calypr/data-client/client/api"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/logs"
	"github.com/calypr/data-client/client/request"
)

type fakeGen3Download struct {
	cred   *conf.Credential
	logger *logs.TeeLogger
	doFunc func(context.Context, *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeGen3Download) GetCredential() *conf.Credential { return f.cred }
func (f *fakeGen3Download) Logger() *logs.TeeLogger         { return f.logger }
func (f *fakeGen3Download) New(method, url string) *request.RequestBuilder {
	return &request.RequestBuilder{Method: method, Url: url}
}
func (f *fakeGen3Download) Do(ctx context.Context, req *request.RequestBuilder) (*http.Response, error) {
	return f.doFunc(ctx, req)
}
func (f *fakeGen3Download) CheckPrivileges(context.Context) (map[string]any, error) {
	return nil, nil
}
func (f *fakeGen3Download) CheckForShepherdAPI(context.Context) (bool, error) { return false, nil }
func (f *fakeGen3Download) DeleteRecord(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeGen3Download) GetDownloadPresignedUrl(context.Context, string, string) (string, error) {
	return "https://download.example.com/object", nil
}
func (f *fakeGen3Download) ParseFenceURLResponse(resp *http.Response) (api.FenceResponse, error) {
	return (&api.Functions{}).ParseFenceURLResponse(resp)
}
func (f *fakeGen3Download) ExportCredential(context.Context, *conf.Credential) error { return nil }
func (f *fakeGen3Download) NewAccessToken(context.Context) error                     { return nil }

func TestDownloadSingleWithProgressEmitsEvents(t *testing.T) {
	payload := bytes.Repeat([]byte("d"), 64)
	downloadDir := t.TempDir()
	downloadPath := downloadDir + string(os.PathSeparator)

	var events []common.ProgressEvent
	progress := func(event common.ProgressEvent) error {
		events = append(events, event)
		return nil
	}

	fake := &fakeGen3Download{
		cred:   &conf.Credential{APIEndpoint: "https://example.com", AccessToken: "token"},
		logger: logs.NewTeeLogger("", "", io.Discard),
		doFunc: func(_ context.Context, req *request.RequestBuilder) (*http.Response, error) {
			switch {
			case strings.Contains(req.Url, common.IndexdIndexEndpoint):
				return newDownloadJSONResponse(req.Url, `{"file_name":"payload.bin","size":64}`), nil
			case strings.HasPrefix(req.Url, "https://download.example.com/"):
				return newDownloadResponse(req.Url, payload, http.StatusOK), nil
			default:
				return nil, errors.New("unexpected request url: " + req.Url)
			}
		},
	}

	err := DownloadSingleWithProgress(context.Background(), fake, "guid-123", downloadPath, "", "oid-123", progress)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected progress events")
	}
	for i := 1; i < len(events); i++ {
		if events[i].BytesSoFar < events[i-1].BytesSoFar {
			t.Fatalf("bytesSoFar not monotonic: %d then %d", events[i-1].BytesSoFar, events[i].BytesSoFar)
		}
	}
	last := events[len(events)-1]
	if last.BytesSoFar != int64(len(payload)) {
		t.Fatalf("expected final bytesSoFar %d, got %d", len(payload), last.BytesSoFar)
	}
	fullPath := filepath.Join(downloadPath, "payload.bin")
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestDownloadSingleWithProgressFinalizeOnError(t *testing.T) {
	downloadDir := t.TempDir()
	downloadPath := downloadDir + string(os.PathSeparator)

	var events []common.ProgressEvent
	progress := func(event common.ProgressEvent) error {
		events = append(events, event)
		return nil
	}

	fake := &fakeGen3Download{
		cred:   &conf.Credential{APIEndpoint: "https://example.com", AccessToken: "token"},
		logger: logs.NewTeeLogger("", "", io.Discard),
		doFunc: func(_ context.Context, req *request.RequestBuilder) (*http.Response, error) {
			switch {
			case strings.Contains(req.Url, common.IndexdIndexEndpoint):
				return newDownloadJSONResponse(req.Url, `{"file_name":"payload.bin","size":64}`), nil
			case strings.HasPrefix(req.Url, "https://download.example.com/"):
				return newDownloadResponse(req.Url, []byte("short"), http.StatusOK), nil
			default:
				return nil, errors.New("unexpected request url: " + req.Url)
			}
		},
	}

	err := DownloadSingleWithProgress(context.Background(), fake, "guid-123", downloadPath, "", "oid-123", progress)
	if err == nil {
		t.Fatal("expected download error")
	}

	if len(events) == 0 {
		t.Fatal("expected progress events")
	}
	last := events[len(events)-1]
	if last.BytesSoFar != 64 {
		t.Fatalf("expected finalize bytesSoFar 64, got %d", last.BytesSoFar)
	}
}

func newDownloadJSONResponse(rawURL, body string) *http.Response {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		parsedURL = &url.URL{}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    &http.Request{URL: parsedURL},
		Header:     make(http.Header),
	}
}

func newDownloadResponse(rawURL string, payload []byte, status int) *http.Response {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		parsedURL = &url.URL{}
	}
	return &http.Response{
		StatusCode:    status,
		Body:          io.NopCloser(bytes.NewReader(payload)),
		ContentLength: int64(len(payload)),
		Request:       &http.Request{URL: parsedURL},
		Header:        make(http.Header),
	}
}
