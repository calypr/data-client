package local

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/conf"
	drs "github.com/calypr/data-client/drs"
	syclient "github.com/calypr/syfon/client"
)

type Signer struct {
	client    *syclient.Client
	drsClient drs.Client
}

func New(baseURL string, cred *conf.Credential, dc drs.Client) *Signer {
	opts := make([]syclient.Option, 0, 1)
	if cred != nil {
		if token := strings.TrimSpace(cred.AccessToken); token != "" {
			opts = append(opts, syclient.WithBearerToken(token))
		}
	}
	return &Signer{
		client:    syclient.New(baseURL, opts...),
		drsClient: dc,
	}
}

func (d *Signer) Name() string { return "DRS" }

func (d *Signer) DeleteFile(ctx context.Context, guid string) (string, error) {
	return "", fmt.Errorf("DeleteFile not implemented for local DRS signer")
}

func (d *Signer) ResolveDownloadURL(ctx context.Context, guid string, accessID string) (string, error) {
	return drs.ResolveDownloadURL(ctx, d.drsClient, guid, accessID)
}

func (d *Signer) ResolveUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error) {
	res, err := d.client.Data().UploadURL(ctx, syclient.UploadURLRequest{
		FileID:   guid,
		Bucket:   bucket,
		FileName: filename,
	})
	if err != nil {
		return "", err
	}
	return res.URL, nil
}

func (d *Signer) ResolveUploadURLs(ctx context.Context, requests []common.UploadURLResolveRequest) ([]common.UploadURLResolveResponse, error) {
	if len(requests) == 0 {
		return []common.UploadURLResolveResponse{}, nil
	}

	items := make([]syclient.UploadBulkItem, 0, len(requests))
	for _, req := range requests {
		fileID := strings.TrimSpace(req.GUID)
		if fileID == "" {
			fileID = strings.TrimSpace(req.Filename)
		}
		item := syclient.UploadBulkItem{FileId: fileID}
		if req.Bucket != "" {
			item.SetBucket(req.Bucket)
		}
		if req.Filename != "" {
			item.SetFileName(req.Filename)
		}
		items = append(items, item)
	}

	out, err := d.client.Data().UploadBulk(ctx, syclient.UploadBulkRequest{Requests: items})
	if err != nil {
		return nil, err
	}

	results := make([]common.UploadURLResolveResponse, len(requests))
	for i := range requests {
		results[i] = common.UploadURLResolveResponse{
			GUID:     requests[i].GUID,
			Filename: requests[i].Filename,
			Bucket:   requests[i].Bucket,
			Status:   http.StatusBadGateway,
			Error:    "missing result for request",
		}
	}
	for i := range out.GetResults() {
		if i >= len(results) {
			break
		}
		r := out.GetResults()[i]
		results[i].URL = r.GetUrl()
		results[i].Status = int(r.GetStatus())
		results[i].Error = r.GetError()
		if results[i].Status == 0 {
			results[i].Status = http.StatusOK
		}
	}
	return results, nil
}

func (d *Signer) InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (*common.MultipartUploadInit, error) {
	res, err := d.client.Data().MultipartInit(ctx, syclient.MultipartInitRequest{
		GUID:     guid,
		FileName: filename,
		Bucket:   bucket,
	})
	if err != nil {
		return nil, err
	}
	if res.UploadID == "" {
		return nil, fmt.Errorf("server did not return uploadId")
	}
	return &common.MultipartUploadInit{
		GUID:     res.GUID,
		UploadID: res.UploadID,
	}, nil
}

func (d *Signer) GetMultipartUploadURL(ctx context.Context, key string, uploadID string, partNumber int32, bucket string) (string, error) {
	res, err := d.client.Data().MultipartUpload(ctx, syclient.MultipartUploadRequest{
		Key:        key,
		Bucket:     bucket,
		UploadID:   uploadID,
		PartNumber: partNumber,
	})
	if err != nil {
		return "", err
	}
	if res.PresignedURL == "" {
		return "", fmt.Errorf("server did not return presigned_url")
	}
	return res.PresignedURL, nil
}

func (d *Signer) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []common.MultipartUploadPart, bucket string) error {
	reqParts := make([]syclient.MultipartPart, len(parts))
	for i, p := range parts {
		reqParts[i] = syclient.MultipartPart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		}
	}
	return d.client.Data().MultipartComplete(ctx, syclient.MultipartCompleteRequest{
		Key:      key,
		Bucket:   bucket,
		UploadID: uploadID,
		Parts:    reqParts,
	})
}
