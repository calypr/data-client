package transfer

import (
	"context"
	"io"
	"net/http"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
)

// Signer defines mode-specific signed URL and multipart orchestration.
type Signer interface {
	Name() string
	ResolveDownloadURL(ctx context.Context, guid string, accessID string) (string, error)
	ResolveUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error)
	ResolveUploadURLs(ctx context.Context, requests []common.UploadURLResolveRequest) ([]common.UploadURLResolveResponse, error)
	InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (*common.MultipartUploadInit, error)
	GetMultipartUploadURL(ctx context.Context, key string, uploadID string, partNumber int32, bucket string) (string, error)
	CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []common.MultipartUploadPart, bucket string) error
	DeleteFile(ctx context.Context, guid string) (string, error)
}

type client struct {
	req    request.RequestInterface
	logger *logs.Gen3Logger
	signer Signer
}

func New(req request.RequestInterface, logger *logs.Gen3Logger, signer Signer) Backend {
	return &client{
		req:    req,
		logger: logger,
		signer: signer,
	}
}

func (c *client) Name() string             { return c.signer.Name() }
func (c *client) Logger() *logs.Gen3Logger { return c.logger }

func (c *client) DeleteFile(ctx context.Context, guid string) (string, error) {
	return c.signer.DeleteFile(ctx, guid)
}

func (c *client) Download(ctx context.Context, fdr *common.FileDownloadResponseObject) (*http.Response, error) {
	return GenericDownload(ctx, c.req, fdr)
}

func (c *client) ResolveDownloadURL(ctx context.Context, guid string, accessID string) (string, error) {
	return c.signer.ResolveDownloadURL(ctx, guid, accessID)
}

func (c *client) ResolveUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error) {
	return c.signer.ResolveUploadURL(ctx, guid, filename, metadata, bucket)
}

func (c *client) ResolveUploadURLs(ctx context.Context, requests []common.UploadURLResolveRequest) ([]common.UploadURLResolveResponse, error) {
	return c.signer.ResolveUploadURLs(ctx, requests)
}

func (c *client) InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (*common.MultipartUploadInit, error) {
	return c.signer.InitMultipartUpload(ctx, guid, filename, bucket)
}

func (c *client) GetMultipartUploadURL(ctx context.Context, key string, uploadID string, partNumber int32, bucket string) (string, error) {
	return c.signer.GetMultipartUploadURL(ctx, key, uploadID, partNumber, bucket)
}

func (c *client) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []common.MultipartUploadPart, bucket string) error {
	return c.signer.CompleteMultipartUpload(ctx, key, uploadID, parts, bucket)
}

func (c *client) Upload(ctx context.Context, url string, body io.Reader, size int64) error {
	_, err := DoUpload(ctx, c.req, url, body, size)
	return err
}

func (c *client) UploadPart(ctx context.Context, url string, body io.Reader, size int64) (string, error) {
	return DoUpload(ctx, c.req, url, body, size)
}
