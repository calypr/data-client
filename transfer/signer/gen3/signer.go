package gen3

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/fence"
	"github.com/calypr/data-client/request"
	syclient "github.com/calypr/syfon/client"
)

type Signer struct {
	req   request.RequestInterface
	cred  *conf.Credential
	drs   drs.Client
	fence fence.FenceInterface
	sy    *syclient.Client
}

func New(req request.RequestInterface, cred *conf.Credential, dc drs.Client, fc fence.FenceInterface) *Signer {
	opts := make([]syclient.Option, 0, 1)
	baseURL := ""
	if cred != nil {
		baseURL = cred.APIEndpoint
		if token := strings.TrimSpace(cred.AccessToken); token != "" {
			opts = append(opts, syclient.WithBearerToken(token))
		}
	}
	return &Signer{
		req:   req,
		cred:  cred,
		drs:   dc,
		fence: fc,
		sy:    syclient.New(baseURL, opts...),
	}
}

func (g *Signer) Name() string {
	return "Gen3"
}

func (g *Signer) DeleteFile(ctx context.Context, guid string) (string, error) {
	return g.fence.DeleteRecord(ctx, guid)
}

func (g *Signer) ResolveDownloadURL(ctx context.Context, guid string, accessID string) (string, error) {
	url, err := g.fence.GetDownloadPresignedUrl(ctx, guid, accessID)
	if err == nil && url != "" {
		return url, nil
	}
	resolved, errIdx := drs.ResolveDownloadURL(ctx, g.drs, guid, accessID)
	if errIdx == nil {
		return resolved, nil
	}
	if err != nil {
		return "", err
	}
	return "", errIdx
}

func (g *Signer) ResolveUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error) {
	hasShepherd, err := g.fence.CheckForShepherdAPI(ctx)
	if err != nil || !hasShepherd {
		var msg fence.FenceResponse
		if guid != "" {
			msg, err = g.fence.GetUploadPresignedUrl(ctx, guid, filename, bucket)
		} else {
			msg, err = g.fence.InitUpload(ctx, filename, bucket, "")
		}
		if err != nil {
			return "", err
		}
		if msg.URL == "" {
			return "", fmt.Errorf("error generating presigned upload URL for %s", filename)
		}
		return msg.URL, nil
	}

	payload := common.ShepherdInitRequestObject{
		Filename: filename,
		Authz: common.ShepherdAuthz{
			Version: "0", ResourcePaths: metadata.Authz,
		},
		Aliases:  metadata.Aliases,
		Metadata: metadata.Metadata,
	}
	reader, err := common.ToJSONReader(payload)
	if err != nil {
		return "", err
	}

	resp, err := g.fence.Do(ctx, &request.RequestBuilder{
		Url:    g.cred.APIEndpoint + common.ShepherdEndpoint + "/objects",
		Method: http.MethodPost,
		Body:   reader,
		Token:  g.cred.AccessToken,
	})
	if err != nil {
		return "", fmt.Errorf("shepherd upload init failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("shepherd upload init failed with status %d", resp.StatusCode)
	}

	var res common.PresignedURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.URL, nil
}

func (g *Signer) ResolveUploadURLs(ctx context.Context, requests []common.UploadURLResolveRequest) ([]common.UploadURLResolveResponse, error) {
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

	out, err := g.sy.Data().UploadBulk(ctx, syclient.UploadBulkRequest{Requests: items})
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

func (g *Signer) InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (*common.MultipartUploadInit, error) {
	res, err := g.fence.InitMultipartUpload(ctx, filename, bucket, guid)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(res.UploadID) == "" {
		return nil, fmt.Errorf("fence multipart init did not return uploadId")
	}
	return &common.MultipartUploadInit{GUID: res.GUID, UploadID: res.UploadID}, nil
}

func (g *Signer) GetMultipartUploadURL(ctx context.Context, key string, uploadID string, partNumber int32, bucket string) (string, error) {
	return g.fence.GenerateMultipartPresignedURL(ctx, key, uploadID, int(partNumber), bucket)
}

func (g *Signer) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []common.MultipartUploadPart, bucket string) error {
	fParts := make([]fence.MultipartPart, len(parts))
	for i, p := range parts {
		fParts[i] = fence.MultipartPart{PartNumber: int(p.PartNumber), ETag: p.ETag}
	}
	return g.fence.CompleteMultipartUpload(ctx, key, uploadID, fParts, bucket)
}
