package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/fence"
	"github.com/calypr/data-client/indexd"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
	"github.com/calypr/data-client/requestor"
	"github.com/calypr/data-client/sower"
)

type fakeGen3Upload struct {
	cred   *conf.Credential
	logger *logs.Gen3Logger
	doFunc func(context.Context, *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeGen3Upload) GetCredential() *conf.Credential { return f.cred }
func (f *fakeGen3Upload) Logger() *logs.Gen3Logger        { return f.logger }
func (f *fakeGen3Upload) ExportCredential(ctx context.Context, cred *conf.Credential) error {
	return nil
}
func (f *fakeGen3Upload) Fence() fence.FenceInterface             { return &fakeFence{doFunc: f.doFunc} }
func (f *fakeGen3Upload) Indexd() indexd.IndexdInterface          { return &fakeIndexd{doFunc: f.doFunc} }
func (f *fakeGen3Upload) Sower() sower.SowerInterface             { return nil }
func (f *fakeGen3Upload) Requestor() requestor.RequestorInterface { return nil }

type fakeFence struct {
	fence.FenceInterface
	doFunc func(context.Context, *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeFence) Do(ctx context.Context, req *request.RequestBuilder) (*http.Response, error) {
	return f.doFunc(ctx, req)
}
func (f *fakeFence) InitMultipartUpload(ctx context.Context, filename string, bucket string, guid string) (fence.FenceResponse, error) {
	resp, err := f.Do(ctx, &request.RequestBuilder{Url: common.FenceDataMultipartInitEndpoint})
	if err != nil {
		return fence.FenceResponse{}, err
	}
	return f.ParseFenceURLResponse(resp)
}
func (f *fakeFence) GenerateMultipartPresignedURL(ctx context.Context, key string, uploadID string, partNumber int, bucket string) (string, error) {
	resp, err := f.Do(ctx, &request.RequestBuilder{Url: common.FenceDataMultipartUploadEndpoint})
	if err != nil {
		return "", err
	}
	msg, err := f.ParseFenceURLResponse(resp)
	return msg.PresignedURL, err
}
func (f *fakeFence) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []fence.MultipartPart, bucket string) error {
	_, err := f.Do(ctx, &request.RequestBuilder{Url: common.FenceDataMultipartCompleteEndpoint})
	return err
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

	logger := logs.NewGen3Logger(nil, "", "")
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
		SourcePath: file.Name(),
		ObjectKey:  "multipart.bin",
		GUID:       "guid-123",
		Bucket:     "bucket",
	}

	ctx = common.WithProgress(ctx, progress)
	ctx = common.WithOid(ctx, "guid-123")

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
