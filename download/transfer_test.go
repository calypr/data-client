package download

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/logs"
)

type fakeBackend struct {
	logger *logs.Gen3Logger
	doFunc func(context.Context, *common.FileDownloadResponseObject) (*http.Response, error)
	data   []byte
}

func (f *fakeBackend) Name() string         { return "Fake" }
func (f *fakeBackend) Logger() *slog.Logger { return f.logger.Logger }

func (f *fakeBackend) GetFileDetails(ctx context.Context, guid string) (*drs.DRSObject, error) {
	return &drs.DRSObject{
		Name: "payload.bin",
		Size: 64,
		AccessMethods: []drs.AccessMethod{
			{AccessID: "s3", Type: "s3"},
		},
	}, nil
}

func (f *fakeBackend) GetDownloadURL(ctx context.Context, guid string, accessID string) (string, error) {
	if guid == "test-fallback" {
		return "", errors.New("fallback")
	}
	return "https://download.example.com/object", nil
}

func (f *fakeBackend) Register(ctx context.Context, obj *drs.DRSObject) (*drs.DRSObject, error) {
	return obj, nil
}

func (f *fakeBackend) BatchRegister(ctx context.Context, objs []*drs.DRSObject) ([]*drs.DRSObject, error) {
	return objs, nil
}

func (f *fakeBackend) GetUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeBackend) InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (*common.MultipartUploadInit, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeBackend) GetMultipartUploadURL(ctx context.Context, key string, uploadID string, partNumber int32, bucket string) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeBackend) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []common.MultipartUploadPart, bucket string) error {
	return errors.New("not implemented")
}

func (f *fakeBackend) Upload(ctx context.Context, url string, body io.Reader, size int64) error {
	return errors.New("not implemented")
}

func (f *fakeBackend) UploadPart(ctx context.Context, url string, body io.Reader, size int64) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeBackend) Download(ctx context.Context, fdr *common.FileDownloadResponseObject) (*http.Response, error) {
	if f.doFunc != nil {
		return f.doFunc(ctx, fdr)
	}
	if fdr.RangeStart != nil && fdr.RangeEnd != nil {
		start, end := *fdr.RangeStart, *fdr.RangeEnd
		if start < 0 || end >= int64(len(f.data)) || start > end {
			return nil, errors.New("invalid range")
		}
		return newDownloadResponse(fdr.PresignedURL, f.data[start:end+1], http.StatusPartialContent), nil
	}
	return newDownloadResponse(fdr.PresignedURL, f.data, http.StatusOK), nil
}

func (f *fakeBackend) GetObjectByHash(ctx context.Context, checksumType, checksum string) ([]drs.DRSObject, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeBackend) BatchGetObjectsByHash(ctx context.Context, hashes []string) (map[string][]drs.DRSObject, error) {
	return nil, errors.New("not implemented")
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

	fake := &fakeBackend{
		logger: logs.NewGen3Logger(nil, "", ""),
		data:   payload,
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

	fake := &fakeBackend{
		logger: logs.NewGen3Logger(nil, "", ""),
		data:   []byte("short"),
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

func TestDownloadToPathMultipart(t *testing.T) {
	payload := bytes.Repeat([]byte("z"), 2*1024*1024) // 2MB
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "multipart.bin")

	fake := &fakeBackend{
		logger: logs.NewGen3Logger(nil, "", ""),
		data:   payload,
	}

	err := DownloadToPathWithOptions(
		context.Background(),
		fake,
		fake.Logger(),
		"guid-789",
		dst,
		"",
		DownloadOptions{
			MultipartThreshold: 1 * 1024 * 1024,
			ChunkSize:          256 * 1024,
			Concurrency:        4,
		},
	)
	if err != nil {
		t.Fatalf("multipart download failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Equal(payload, got) {
		t.Fatal("downloaded payload mismatch")
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
