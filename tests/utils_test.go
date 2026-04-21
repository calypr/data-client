package tests

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/calypr/data-client/common"
	sylogs "github.com/calypr/syfon/client/pkg/logs"
	"github.com/calypr/syfon/client/xfer/download"
	"github.com/calypr/syfon/client/xfer/upload"
)

type fakeDownloader struct {
	resolveFn  func(ctx context.Context, guid, accessID string) (string, error)
	downloadFn func(ctx context.Context, fdr *common.FileDownloadResponseObject) (*http.Response, error)
}

func (f *fakeDownloader) Name() string               { return "fake-downloader" }
func (f *fakeDownloader) Logger() *sylogs.Gen3Logger { return sylogs.NewGen3Logger(nil, "", "test") }
func (f *fakeDownloader) ResolveDownloadURL(ctx context.Context, guid, accessID string) (string, error) {
	return f.resolveFn(ctx, guid, accessID)
}
func (f *fakeDownloader) Download(ctx context.Context, fdr *common.FileDownloadResponseObject) (*http.Response, error) {
	return f.downloadFn(ctx, fdr)
}

type fakeUploader struct {
	resolveFn func(ctx context.Context, guid, filename string, metadata common.FileMetadata, bucket string) (string, error)
}

func (f *fakeUploader) Name() string               { return "fake-uploader" }
func (f *fakeUploader) Logger() *sylogs.Gen3Logger { return sylogs.NewGen3Logger(nil, "", "test") }

func (f *fakeUploader) ResolveUploadURL(ctx context.Context, guid, filename string, metadata common.FileMetadata, bucket string) (string, error) {
	return f.resolveFn(ctx, guid, filename, metadata, bucket)
}
func (f *fakeUploader) ResolveUploadURLs(ctx context.Context, requests []common.UploadURLResolveRequest) ([]common.UploadURLResolveResponse, error) {
	return nil, nil
}
func (f *fakeUploader) InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (*common.MultipartUploadInit, error) {
	return nil, nil
}
func (f *fakeUploader) GetMultipartUploadURL(ctx context.Context, key string, uploadID string, partNumber int32, bucket string) (string, error) {
	return "", nil
}
func (f *fakeUploader) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []common.MultipartUploadPart, bucket string) error {
	return nil
}
func (f *fakeUploader) Upload(ctx context.Context, url string, body io.Reader, size int64) error {
	return nil
}
func (f *fakeUploader) UploadPart(ctx context.Context, url string, body io.Reader, size int64) (string, error) {
	return "", nil
}
func (f *fakeUploader) DeleteFile(ctx context.Context, guid string) (string, error) {
	return "", nil
}

func TestGetDownloadResponse(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFilename := "test-file"
	mockDownloadURL := "https://example.com/example.pfb"

	bk := &fakeDownloader{
		resolveFn: func(ctx context.Context, guid, accessID string) (string, error) {
			if guid != testGUID {
				t.Fatalf("unexpected guid: %s", guid)
			}
			return mockDownloadURL, nil
		},
		downloadFn: func(ctx context.Context, fdr *common.FileDownloadResponseObject) (*http.Response, error) {
			if fdr.PresignedURL != mockDownloadURL {
				t.Fatalf("expected URL %s, got %s", mockDownloadURL, fdr.PresignedURL)
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("content")),
			}, nil
		},
	}

	mockFDRObj := common.FileDownloadResponseObject{
		Filename: testFilename,
		GUID:     testGUID,
		Range:    0,
	}

	err := download.GetDownloadResponse(context.Background(), bk, &mockFDRObj, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockFDRObj.PresignedURL != mockDownloadURL {
		t.Errorf("wanted URL %s, got %s", mockDownloadURL, mockFDRObj.PresignedURL)
	}
}

func TestGeneratePresignedUploadURL(t *testing.T) {
	testFilename := "test-file"
	testBucket := "test-bucket"
	mockUploadURL := "https://example.com/upload"

	bk := &fakeUploader{
		resolveFn: func(ctx context.Context, guid, filename string, metadata common.FileMetadata, bucket string) (string, error) {
			if filename != testFilename {
				t.Fatalf("unexpected filename: %s", filename)
			}
			if bucket != testBucket {
				t.Fatalf("unexpected bucket: %s", bucket)
			}
			return mockUploadURL, nil
		},
	}

	resp, err := upload.GeneratePresignedUploadURL(context.Background(), bk, testFilename, common.FileMetadata{}, testBucket)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.URL != mockUploadURL {
		t.Errorf("wanted URL %s, got %s", mockUploadURL, resp.URL)
	}
	if resp.GUID != "" {
		t.Errorf("wanted empty GUID, got %s", resp.GUID)
	}
}
