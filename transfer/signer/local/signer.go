package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/calypr/data-client/common"
	drs "github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/request"
)

type Signer struct {
	baseURL string
	req     request.RequestInterface
	client  drs.Client
}

type bulkUploadRequest struct {
	Requests []bulkUploadItem `json:"requests"`
}

type bulkUploadItem struct {
	FileID   string `json:"file_id"`
	Bucket   string `json:"bucket,omitempty"`
	FileName string `json:"file_name,omitempty"`
}

type bulkUploadResponse struct {
	Results []bulkUploadResult `json:"results"`
}

type bulkUploadResult struct {
	FileID   string `json:"file_id"`
	Bucket   string `json:"bucket,omitempty"`
	FileName string `json:"file_name,omitempty"`
	URL      string `json:"url,omitempty"`
	Status   int    `json:"status"`
	Error    string `json:"error,omitempty"`
}

func New(baseURL string, req request.RequestInterface, dc drs.Client) *Signer {
	return &Signer{
		baseURL: baseURL,
		req:     req,
		client:  dc,
	}
}

func (d *Signer) Name() string { return "DRS" }

func (d *Signer) DeleteFile(ctx context.Context, guid string) (string, error) {
	return "", fmt.Errorf("DeleteFile not implemented for local DRS signer")
}

func (d *Signer) buildURL(paths ...string) (string, error) {
	u, err := url.Parse(d.baseURL)
	if err != nil {
		return "", err
	}
	fullPath := u.Path
	for _, p := range paths {
		if p == "" {
			continue
		}
		if !strings.HasSuffix(fullPath, "/") && !strings.HasPrefix(p, "/") {
			fullPath += "/"
		}
		fullPath += p
	}
	u.Path = fullPath
	return u.String(), nil
}

func (d *Signer) doJSONRequest(ctx context.Context, method, url string, body interface{}, dst interface{}) error {
	rb := d.req.New(method, url)
	if body != nil {
		if _, err := rb.WithJSONBody(body); err != nil {
			return err
		}
	}

	resp, err := d.req.Do(ctx, rb)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request to %s failed with status %d: %s", url, resp.StatusCode, string(bodyBytes))
	}

	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}

func (d *Signer) ResolveDownloadURL(ctx context.Context, guid string, accessID string) (string, error) {
	return drs.ResolveDownloadURL(ctx, d.client, guid, accessID)
}

func (d *Signer) ResolveUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error) {
	u, err := d.buildURL("data/upload", guid)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	if strings.TrimSpace(filename) != "" {
		q.Set("file_name", filename)
	}
	if bucket != "" {
		q.Set("bucket", bucket)
	}
	if encoded := q.Encode(); encoded != "" {
		u += "?" + encoded
	}

	var res struct {
		URL string `json:"url"`
	}
	if err := d.doJSONRequest(ctx, http.MethodGet, u, nil, &res); err != nil {
		return "", err
	}
	return res.URL, nil
}

func (d *Signer) ResolveUploadURLs(ctx context.Context, requests []common.UploadURLResolveRequest) ([]common.UploadURLResolveResponse, error) {
	if len(requests) == 0 {
		return []common.UploadURLResolveResponse{}, nil
	}

	u, err := d.buildURL("data/upload/bulk")
	if err != nil {
		return nil, err
	}

	payload := bulkUploadRequest{Requests: make([]bulkUploadItem, 0, len(requests))}
	for _, req := range requests {
		fileID := strings.TrimSpace(req.GUID)
		if fileID == "" {
			fileID = strings.TrimSpace(req.Filename)
		}
		payload.Requests = append(payload.Requests, bulkUploadItem{
			FileID:   fileID,
			Bucket:   req.Bucket,
			FileName: req.Filename,
		})
	}

	var out bulkUploadResponse
	if err := d.doJSONRequest(ctx, http.MethodPost, u, payload, &out); err != nil {
		return nil, err
	}

	results := make([]common.UploadURLResolveResponse, len(requests))
	if len(out.Results) == len(requests) {
		for i := range requests {
			r := out.Results[i]
			results[i] = common.UploadURLResolveResponse{
				GUID:     requests[i].GUID,
				Filename: requests[i].Filename,
				Bucket:   requests[i].Bucket,
				URL:      r.URL,
				Status:   r.Status,
				Error:    r.Error,
			}
			if results[i].Status == 0 {
				results[i].Status = http.StatusOK
			}
		}
		return results, nil
	}

	// If response count mismatches, align by request order and mark unresolved entries.
	for i := range requests {
		results[i] = common.UploadURLResolveResponse{
			GUID:     requests[i].GUID,
			Filename: requests[i].Filename,
			Bucket:   requests[i].Bucket,
			Status:   http.StatusBadGateway,
			Error:    "missing result for request",
		}
	}
	for i := range out.Results {
		if i >= len(results) {
			break
		}
		r := out.Results[i]
		results[i].URL = r.URL
		results[i].Status = r.Status
		results[i].Error = r.Error
		if results[i].Status == 0 {
			results[i].Status = http.StatusOK
		}
	}
	return results, nil
}

func (d *Signer) InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (*common.MultipartUploadInit, error) {
	u, err := d.buildURL("data/multipart/init")
	if err != nil {
		return nil, err
	}

	req := struct {
		GUID     string `json:"guid,omitempty"`
		FileName string `json:"file_name,omitempty"`
		Bucket   string `json:"bucket,omitempty"`
	}{
		GUID:     guid,
		FileName: filename,
		Bucket:   bucket,
	}

	var res struct {
		GUID     string `json:"guid"`
		UploadID string `json:"uploadId"`
	}
	if err := d.doJSONRequest(ctx, http.MethodPost, u, req, &res); err != nil {
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
	u, err := d.buildURL("data/multipart/upload")
	if err != nil {
		return "", err
	}

	req := struct {
		Key        string `json:"key"`
		Bucket     string `json:"bucket,omitempty"`
		UploadID   string `json:"uploadId"`
		PartNumber int32  `json:"partNumber"`
	}{
		Key:        key,
		Bucket:     bucket,
		UploadID:   uploadID,
		PartNumber: partNumber,
	}

	var res struct {
		PresignedURL string `json:"presigned_url"`
	}
	if err := d.doJSONRequest(ctx, http.MethodPost, u, req, &res); err != nil {
		return "", err
	}
	if res.PresignedURL == "" {
		return "", fmt.Errorf("server did not return presigned_url")
	}
	return res.PresignedURL, nil
}

func (d *Signer) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []common.MultipartUploadPart, bucket string) error {
	u, err := d.buildURL("data/multipart/complete")
	if err != nil {
		return err
	}

	reqParts := make([]struct {
		PartNumber int32  `json:"PartNumber"`
		ETag       string `json:"ETag"`
	}, len(parts))
	for i, p := range parts {
		reqParts[i] = struct {
			PartNumber int32  `json:"PartNumber"`
			ETag       string `json:"ETag"`
		}{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		}
	}

	req := struct {
		Key      string `json:"key"`
		Bucket   string `json:"bucket,omitempty"`
		UploadID string `json:"uploadId"`
		Parts    any    `json:"parts"`
	}{
		Key:      key,
		Bucket:   bucket,
		UploadID: uploadID,
		Parts:    reqParts,
	}

	return d.doJSONRequest(ctx, http.MethodPost, u, req, nil)
}
