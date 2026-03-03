package gen3

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/calypr/data-client/backend"
	"github.com/calypr/data-client/common"
	drs "github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/fence"
	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/request"
)

type Gen3Backend struct {
	client g3client.Gen3Interface
}

func NewGen3Backend(client g3client.Gen3Interface) backend.Backend {
	return &Gen3Backend{
		client: client,
	}
}

func (g *Gen3Backend) Name() string {
	return "Gen3"
}

func (g *Gen3Backend) Logger() *slog.Logger {
	return g.client.Logger().Logger
}

func (g *Gen3Backend) Download(ctx context.Context, fdr *common.FileDownloadResponseObject) (*http.Response, error) {
	skipAuth := common.IsCloudPresignedURL(fdr.PresignedURL)

	rb := g.client.New(http.MethodGet, fdr.PresignedURL)
	start, end, hasRange := resolveRange(fdr)
	if hasRange {
		rangeHeader := "bytes=" + strconv.FormatInt(start, 10) + "-"
		if end != nil {
			rangeHeader += strconv.FormatInt(*end, 10)
		}
		rb.WithHeader("Range", rangeHeader)
	}

	if skipAuth {
		rb.WithSkipAuth(true)
	}

	return g.client.Do(ctx, rb)
}

func resolveRange(fdr *common.FileDownloadResponseObject) (start int64, end *int64, ok bool) {
	if fdr == nil {
		return 0, nil, false
	}
	if fdr.RangeStart != nil {
		return *fdr.RangeStart, fdr.RangeEnd, true
	}
	if fdr.Range > 0 {
		return fdr.Range, nil, true
	}
	return 0, nil, false
}

func (g *Gen3Backend) GetFileDetails(ctx context.Context, guid string) (*drs.DRSObject, error) {
	// 1. Try Shepherd
	hasShepherd, err := g.client.Fence().CheckForShepherdAPI(ctx)
	if err == nil && hasShepherd {
		endpoint := strings.TrimSuffix(g.client.GetCredential().APIEndpoint, "/") + common.ShepherdEndpoint + "/objects/" + guid
		rb := g.client.New(http.MethodGet, endpoint)
		resp, err := g.client.Do(ctx, rb)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var shepherdResp struct {
				Record struct {
					FileName string `json:"file_name"`
					Size     int64  `json:"size"`
					Did      string `json:"did"`
				} `json:"record"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&shepherdResp); err == nil {
				return &drs.DRSObject{
					Name: shepherdResp.Record.FileName,
					Size: shepherdResp.Record.Size,
					Id:   shepherdResp.Record.Did,
				}, nil
			}
		}
		if err != nil {
			g.Logger().Warn("Shepherd lookup failed, falling back to Indexd", "guid", guid, "error", err)
		}
	}

	// 2. Fallback to Indexd
	return g.client.Indexd().GetObject(ctx, guid)
}

func (g *Gen3Backend) GetObjectByHash(ctx context.Context, checksumType, checksum string) ([]drs.DRSObject, error) {
	return g.client.Indexd().GetObjectByHash(ctx, checksumType, checksum)
}

func (g *Gen3Backend) BatchGetObjectsByHash(ctx context.Context, hashes []string) (map[string][]drs.DRSObject, error) {
	return g.client.Indexd().BatchGetObjectsByHash(ctx, hashes)
}

func (g *Gen3Backend) GetDownloadURL(ctx context.Context, guid string, accessID string) (string, error) {
	// For Gen3, often "accessID" is used as a protocol hint like "s3", "gs", or "?protocol=s3"
	// 1. Try Fence first
	url, err := g.client.Fence().GetDownloadPresignedUrl(ctx, guid, accessID)
	if err == nil && url != "" {
		return url, nil
	}

	// 2. Fallback to Indexd
	// Indexd expects "s3", "gs", "ftp", "http", "https" etc.
	// We need to clean up accessID if it contains query params like "?protocol="
	accessType := "s3" // default
	if strings.Contains(accessID, "protocol=") {
		parts := strings.Split(accessID, "=")
		if len(parts) > 1 {
			accessType = parts[len(parts)-1]
		}
	} else if accessID != "" {
		accessType = accessID
	}

	resp, errIdx := g.client.Indexd().GetDownloadURL(ctx, guid, accessType)
	if errIdx == nil && resp != nil && resp.URL != "" {
		return resp.URL, nil
	}

	if err != nil {
		return "", err
	}
	if errIdx != nil {
		return "", errIdx
	}
	return "", fmt.Errorf("failed to resolve download URL for %s", guid)
}

func (g *Gen3Backend) Register(ctx context.Context, obj *drs.DRSObject) (*drs.DRSObject, error) {
	return g.client.Indexd().RegisterRecord(ctx, obj)
}

func (g *Gen3Backend) BatchRegister(ctx context.Context, objs []*drs.DRSObject) ([]*drs.DRSObject, error) {
	return g.client.Indexd().RegisterRecords(ctx, objs)
}

// ShepherdInitRequestObject copied from upload/types.go to avoid circular dependency
type ShepherdInitRequestObject struct {
	Filename string         `json:"file_name"`
	Authz    ShepherdAuthz  `json:"authz"`
	Aliases  []string       `json:"aliases"`
	Metadata map[string]any `json:"metadata"`
}

type ShepherdAuthz struct {
	Version       string   `json:"version"`
	ResourcePaths []string `json:"resource_paths"`
}

type PresignedURLResponse struct {
	GUID string `json:"guid"`
	URL  string `json:"upload_url"`
}

func (g *Gen3Backend) GetUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error) {
	hasShepherd, err := g.client.Fence().CheckForShepherdAPI(ctx)
	if err != nil || !hasShepherd {
		// Fallback to Fence
		var msg fence.FenceResponse

		if guid != "" {
			msg, err = g.client.Fence().GetUploadPresignedUrl(ctx, guid, filename, bucket)
		} else {
			// Init upload if no GUID
			msg, err = g.client.Fence().InitUpload(ctx, filename, bucket, "")
		}

		if err != nil {
			return "", err
		}
		if msg.URL == "" {
			return "", fmt.Errorf("error in generating presigned URL for %s", filename)
		}
		return msg.URL, nil
	}

	// Shepherd Logic
	shepherdPayload := ShepherdInitRequestObject{
		Filename: filename,
		Authz: ShepherdAuthz{
			Version: "0", ResourcePaths: metadata.Authz,
		},
		Aliases:  metadata.Aliases,
		Metadata: metadata.Metadata,
	}

	reader, err := common.ToJSONReader(shepherdPayload)
	if err != nil {
		return "", err
	}

	cred := g.client.GetCredential()
	r, err := g.client.Fence().Do(
		ctx,
		&request.RequestBuilder{
			Url:    cred.APIEndpoint + common.ShepherdEndpoint + "/objects",
			Method: http.MethodPost,
			Body:   reader,
			Token:  cred.AccessToken,
		})
	if err != nil {
		return "", fmt.Errorf("shepherd upload init failed: %w", err)
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusCreated && r.StatusCode != http.StatusOK {
		return "", fmt.Errorf("shepherd upload init failed with status %d", r.StatusCode)
	}

	var res PresignedURLResponse
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.URL, nil
}

func (g *Gen3Backend) InitMultipartUpload(ctx context.Context, guid string, filename string, bucket string) (*common.MultipartUploadInit, error) {
	res, err := g.client.Fence().InitMultipartUpload(ctx, filename, bucket, guid)
	if err != nil {
		return nil, err
	}
	if res.UploadID == "" {
		return nil, fmt.Errorf("fence multipart init did not return uploadId")
	}
	return &common.MultipartUploadInit{
		GUID:     res.GUID,
		UploadID: res.UploadID,
	}, nil
}

func (g *Gen3Backend) GetMultipartUploadURL(ctx context.Context, key string, uploadID string, partNumber int32, bucket string) (string, error) {
	return g.client.Fence().GenerateMultipartPresignedURL(ctx, key, uploadID, int(partNumber), bucket)
}

func (g *Gen3Backend) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []common.MultipartUploadPart, bucket string) error {
	fParts := make([]fence.MultipartPart, len(parts))
	for i, p := range parts {
		fParts[i] = fence.MultipartPart{
			PartNumber: int(p.PartNumber),
			ETag:       p.ETag,
		}
	}
	return g.client.Fence().CompleteMultipartUpload(ctx, key, uploadID, fParts, bucket)
}

func (g *Gen3Backend) doUpload(ctx context.Context, url string, body io.Reader, size int64) (string, error) {
	rb := g.client.New(http.MethodPut, url).
		WithBody(body).
		WithSkipAuth(true)
	if size > 0 {
		rb.PartSize = size
	}

	resp, err := g.client.Do(ctx, rb)
	if err != nil {
		return "", fmt.Errorf("upload to %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload to %s failed with status %d: %s", url, resp.StatusCode, string(bodyBytes))
	}

	return strings.Trim(resp.Header.Get("ETag"), `"`), nil
}

func (g *Gen3Backend) Upload(ctx context.Context, url string, body io.Reader, size int64) error {
	_, err := g.doUpload(ctx, url, body, size)
	return err
}

func (g *Gen3Backend) UploadPart(ctx context.Context, url string, body io.Reader, size int64) (string, error) {
	etag, err := g.doUpload(ctx, url, body, size)
	if err != nil {
		return "", err
	}
	if etag == "" {
		return "", fmt.Errorf("multipart upload part returned empty ETag")
	}
	return etag, nil
}
