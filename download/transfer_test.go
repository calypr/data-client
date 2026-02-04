package download

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/fence"
	"github.com/calypr/data-client/indexd"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
	"github.com/calypr/data-client/requestor"
	"github.com/calypr/data-client/sower"
)

type fakeGen3Download struct {
	cred   *conf.Credential
	logger *logs.Gen3Logger
	doFunc func(context.Context, *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeGen3Download) GetCredential() *conf.Credential { return f.cred }
func (f *fakeGen3Download) Logger() *logs.Gen3Logger        { return f.logger }
func (f *fakeGen3Download) ExportCredential(ctx context.Context, cred *conf.Credential) error {
	return nil
}
func (f *fakeGen3Download) Fence() fence.FenceInterface             { return &fakeFence{doFunc: f.doFunc} }
func (f *fakeGen3Download) Indexd() indexd.IndexdInterface          { return &fakeIndexd{doFunc: f.doFunc} }
func (f *fakeGen3Download) Sower() sower.SowerInterface             { return nil }
func (f *fakeGen3Download) Requestor() requestor.RequestorInterface { return nil }

type fakeFence struct {
	fence.FenceInterface
	doFunc func(context.Context, *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeFence) Do(ctx context.Context, req *request.RequestBuilder) (*http.Response, error) {
	return f.doFunc(ctx, req)
}
func (f *fakeFence) New(method, url string) *request.RequestBuilder {
	return &request.RequestBuilder{Method: method, Url: url, Headers: make(map[string]string)}
}
func (f *fakeFence) CheckForShepherdAPI(ctx context.Context) (bool, error) { return false, nil }
func (f *fakeFence) ResolveOID(ctx context.Context, oid string) (fence.FenceResponse, error) {
	return fence.FenceResponse{}, nil
}
func (f *fakeFence) GetDownloadPresignedUrl(ctx context.Context, guid, protocol string) (string, error) {
	if guid == "test-fallback" {
		return "", errors.New("fence fallback")
	}
	return "https://download.example.com/object", nil
}
func (f *fakeFence) ParseFenceURLResponse(resp *http.Response) (fence.FenceResponse, error) {
	var msg fence.FenceResponse
	if resp != nil && resp.Body != nil {
		json.NewDecoder(resp.Body).Decode(&msg)
	}
	return msg, nil
}

type fakeIndexd struct {
	indexd.IndexdInterface
	doFunc func(context.Context, *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeIndexd) Do(ctx context.Context, req *request.RequestBuilder) (*http.Response, error) {
	return f.doFunc(ctx, req)
}

func (f *fakeIndexd) New(method, url string) *request.RequestBuilder {
	return &request.RequestBuilder{Method: method, Url: url, Headers: make(map[string]string)}
}

func (f *fakeIndexd) GetDownloadURL(ctx context.Context, did string, accessType string) (*drs.AccessURL, error) {
	return &drs.AccessURL{URL: "https://download.example.com/object"}, nil
}

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
		logger: logs.NewGen3Logger(nil, "", ""),
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

	ctx := common.WithProgress(context.Background(), progress)
	err := DownloadSingleWithProgress(ctx, fake, "guid-123", downloadPath, "")
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
		logger: logs.NewGen3Logger(nil, "", ""),
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

	ctx := common.WithProgress(context.Background(), progress)
	err := DownloadSingleWithProgress(ctx, fake, "guid-123", downloadPath, "")
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

// fakeRequestor implements requestor.RequestorInterface using the same doFunc.
type fakeRequestor struct {
	requestor.RequestorInterface
	doFunc func(context.Context, *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeRequestor) Do(ctx context.Context, req *request.RequestBuilder) (*http.Response, error) {
	return f.doFunc(ctx, req)
}

func (f *fakeRequestor) New(method, url string) *request.RequestBuilder {
	return &request.RequestBuilder{Method: method, Url: url, Headers: make(map[string]string)}
}

// Requestor returns a fakeRequestor for the fakeGen3Download.
func (f *fakeGen3Download) Requestor() requestor.RequestorInterface {
	return &fakeRequestor{doFunc: f.doFunc}
}
