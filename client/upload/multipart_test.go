package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/calypr/data-client/client/api"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/logs"
	"github.com/calypr/data-client/client/request"
)

type fakeGen3Upload struct {
	cred   *conf.Credential
	logger *logs.TeeLogger
	doFunc func(context.Context, *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeGen3Upload) GetCredential() *conf.Credential { return f.cred }
func (f *fakeGen3Upload) Logger() *logs.TeeLogger         { return f.logger }
func (f *fakeGen3Upload) New(method, url string) *request.RequestBuilder {
	return &request.RequestBuilder{Method: method, Url: url}
}
func (f *fakeGen3Upload) Do(ctx context.Context, req *request.RequestBuilder) (*http.Response, error) {
	return f.doFunc(ctx, req)
}
func (f *fakeGen3Upload) CheckPrivileges(context.Context) (map[string]any, error) {
	return nil, nil
}
func (f *fakeGen3Upload) CheckForShepherdAPI(context.Context) (bool, error) { return false, nil }
func (f *fakeGen3Upload) DeleteRecord(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeGen3Upload) GetDownloadPresignedUrl(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *fakeGen3Upload) ParseFenceURLResponse(resp *http.Response) (api.FenceResponse, error) {
	return (&api.Functions{}).ParseFenceURLResponse(resp)
}
func (f *fakeGen3Upload) ExportCredential(context.Context, *conf.Credential) error { return nil }
func (f *fakeGen3Upload) NewAccessToken(context.Context) error                     { return nil }

func TestMultipartUploadProgressIntegration(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.Header().Set("ETag", "etag-123")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	file, err := os.CreateTemp(t.TempDir(), "multipart-*.bin")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer file.Close()

	fileSize := int64(101 * common.MB)
	if err := file.Truncate(fileSize); err != nil {
		t.Fatalf("truncate file: %v", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek file: %v", err)
	}

	var (
		events []common.ProgressEvent
		mu     sync.Mutex
	)
	progress := func(event common.ProgressEvent) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
		return nil
	}

	logger := logs.NewTeeLogger("", "", io.Discard)
	fake := &fakeGen3Upload{
		cred: &conf.Credential{
			APIEndpoint: "https://example.com",
			AccessToken: "token",
		},
		logger: logger,
		doFunc: func(_ context.Context, req *request.RequestBuilder) (*http.Response, error) {
			switch {
			case strings.Contains(req.Url, common.FenceDataMultipartInitEndpoint):
				return newJSONResponse(req.Url, `{"uploadId":"upload-123","guid":"guid-123"}`), nil
			case strings.Contains(req.Url, common.FenceDataMultipartUploadEndpoint):
				return newJSONResponse(req.Url, fmt.Sprintf(`{"presigned_url":"%s"}`, server.URL)), nil
			case strings.Contains(req.Url, common.FenceDataMultipartCompleteEndpoint):
				return newJSONResponse(req.Url, `{}`), nil
			default:
				return nil, fmt.Errorf("unexpected request url: %s", req.Url)
			}
		},
	}

	requestObject := common.FileUploadRequestObject{
		FilePath: file.Name(),
		Filename: "multipart.bin",
		GUID:     "guid-123",
		OID:      "oid-123",
		Bucket:   "bucket",
		Progress: progress,
	}

	if err := MultipartUpload(ctx, fake, requestObject, file, false); err != nil {
		t.Fatalf("multipart upload failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("expected progress events")
	}
	for i := 1; i < len(events); i++ {
		if events[i].BytesSoFar < events[i-1].BytesSoFar {
			t.Fatalf("bytesSoFar not monotonic: %d then %d", events[i-1].BytesSoFar, events[i].BytesSoFar)
		}
	}
	last := events[len(events)-1]
	if last.BytesSoFar != fileSize {
		t.Fatalf("expected final bytesSoFar %d, got %d", fileSize, last.BytesSoFar)
	}
}

func newJSONResponse(rawURL, body string) *http.Response {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		parsedURL = &url.URL{}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Request:    &http.Request{URL: parsedURL},
		Header:     make(http.Header),
	}
}
