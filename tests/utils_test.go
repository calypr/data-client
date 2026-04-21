package tests

import (
	"context"
	"io"
	"net/http"
	"testing"

	internalapi "github.com/calypr/syfon/apigen/client/internalapi"
	sycommon "github.com/calypr/syfon/client/common"
	sylogs "github.com/calypr/syfon/client/logs"
	"github.com/calypr/syfon/client/transfer"
	"github.com/calypr/syfon/client/xfer/upload"
)

type fakeDownloader struct {
	resolveFn  func(ctx context.Context, guid, accessID string) (string, error)
	downloadFn func(ctx context.Context, url string, rangeStart, rangeEnd *int64) (*http.Response, error)
}

func (f *fakeDownloader) Name() string { return "fake-downloader" }
func (f *fakeDownloader) Logger() transfer.TransferLogger {
	return sylogs.NewGen3Logger(nil, "", "test")
}
func (f *fakeDownloader) ResolveDownloadURL(ctx context.Context, guid, accessID string) (string, error) {
	return f.resolveFn(ctx, guid, accessID)
}
func (f *fakeDownloader) Download(ctx context.Context, url string, rangeStart, rangeEnd *int64) (*http.Response, error) {
	return f.downloadFn(ctx, url, rangeStart, rangeEnd)
}

type fakeUploader struct {
	resolveFn func(ctx context.Context, guid, filename string, metadata sycommon.FileMetadata, bucket string) (string, error)
}

func (f *fakeUploader) Name() string                    { return "fake-uploader" }
func (f *fakeUploader) Logger() transfer.TransferLogger { return sylogs.NewGen3Logger(nil, "", "test") }

func (f *fakeUploader) ResolveUploadURL(ctx context.Context, guid, filename string, metadata sycommon.FileMetadata, bucket string) (string, error) {
	return f.resolveFn(ctx, guid, filename, metadata, bucket)
}
func (f *fakeUploader) InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (string, string, error) {
	return "", "", nil
}
func (f *fakeUploader) GetMultipartUploadURL(ctx context.Context, key string, uploadID string, partNumber int32, bucket string) (string, error) {
	return "", nil
}
func (f *fakeUploader) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []internalapi.InternalMultipartPart, bucket string) error {
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
func (f *fakeUploader) CanonicalObjectURL(signedURL, bucketHint, fallbackDID string) (string, error) {
	return signedURL, nil
}

func TestGeneratePresignedUploadURL(t *testing.T) {
	testFilename := "test-file"
	testBucket := "test-bucket"
	mockUploadURL := "https://example.com/upload"

	bk := &fakeUploader{
		resolveFn: func(ctx context.Context, guid, filename string, metadata sycommon.FileMetadata, bucket string) (string, error) {
			if filename != testFilename {
				t.Fatalf("unexpected filename: %s", filename)
			}
			if bucket != testBucket {
				t.Fatalf("unexpected bucket: %s", bucket)
			}
			return mockUploadURL, nil
		},
	}

	resp, err := upload.GeneratePresignedUploadURL(context.Background(), bk, testFilename, sycommon.FileMetadata{}, testBucket)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != mockUploadURL {
		t.Errorf("wanted URL %s, got %s", mockUploadURL, resp)
	}
}
