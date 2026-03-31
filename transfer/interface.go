package transfer

import (
	"context"
	"io"
	"net/http"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/logs"
)

// Service captures identity and logging for a transfer implementation.
type Service interface {
	Name() string
	Logger() *logs.Gen3Logger
}

// Downloader is the signed URL resolution and byte download surface.
type Downloader interface {
	Service
	ResolveDownloadURL(ctx context.Context, guid string, accessID string) (string, error)
	Download(ctx context.Context, fdr *common.FileDownloadResponseObject) (*http.Response, error)
}

// Uploader is the signed URL and multipart upload surface.
type Uploader interface {
	Service
	ResolveUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error)
	ResolveUploadURLs(ctx context.Context, requests []common.UploadURLResolveRequest) ([]common.UploadURLResolveResponse, error)
	InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (*common.MultipartUploadInit, error)
	GetMultipartUploadURL(ctx context.Context, key string, uploadID string, partNumber int32, bucket string) (string, error)
	CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []common.MultipartUploadPart, bucket string) error
	Upload(ctx context.Context, url string, body io.Reader, size int64) error
	UploadPart(ctx context.Context, url string, body io.Reader, size int64) (string, error)
	DeleteFile(ctx context.Context, guid string) (string, error)
}

// Backend is the composed transfer surface used by upload/download workflows.
type Backend interface {
	Downloader
	Uploader
}
