package drs_backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/calypr/data-client/backend"
	"github.com/calypr/data-client/common"
	drs "github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/request"
)

type DrsBackend struct {
	BaseURL string
	logger  *slog.Logger
	req     request.RequestInterface
}

func NewDrsBackend(baseURL string, logger *slog.Logger, req request.RequestInterface) backend.Backend {
	return &DrsBackend{
		BaseURL: baseURL,
		logger:  logger,
		req:     req,
	}
}

func (d *DrsBackend) Name() string {
	return "DRS"
}

func (d *DrsBackend) Logger() *slog.Logger {
	return d.logger
}

func (d *DrsBackend) Download(ctx context.Context, fdr *common.FileDownloadResponseObject) (*http.Response, error) {
	skipAuth := common.IsCloudPresignedURL(fdr.PresignedURL)

	rb := d.req.New(http.MethodGet, fdr.PresignedURL)
	if fdr.Range > 0 {
		rb.WithHeader("Range", "bytes="+strconv.FormatInt(fdr.Range, 10)+"-")
	}

	if skipAuth {
		rb.WithSkipAuth(true)
	}

	return d.req.Do(ctx, rb)
}

func (d *DrsBackend) buildURL(paths ...string) (string, error) {
	u, err := url.Parse(d.BaseURL)
	if err != nil {
		return "", err
	}
	// path.Join collapses //, which mangles access_id if it's a URL.
	// We join manually but ensure we don't end up with triple slashes if a part starts/ends with /.
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

func (d *DrsBackend) doJSONRequest(ctx context.Context, method, url string, body interface{}, dst interface{}) error {
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

func (d *DrsBackend) GetFileDetails(ctx context.Context, guid string) (*drs.DRSObject, error) {
	u, err := d.buildURL("ga4gh/drs/v1/objects", guid)
	if err != nil {
		return nil, err
	}

	var obj drs.DRSObject
	if err := d.doJSONRequest(ctx, http.MethodGet, u, nil, &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (d *DrsBackend) GetDownloadURL(ctx context.Context, guid string, accessID string) (string, error) {
	// If accessID is empty, try to find one
	if accessID == "" {
		obj, err := d.GetFileDetails(ctx, guid)
		if err != nil {
			return "", err
		}
		if len(obj.AccessMethods) == 0 {
			return "", fmt.Errorf("no access methods found for object %s", guid)
		}

		// Prefer one with AccessID
		for _, am := range obj.AccessMethods {
			if am.AccessID != "" {
				accessID = am.AccessID
				break
			}
		}
		if accessID == "" {
			// Fallback to first if defined
			if len(obj.AccessMethods) > 0 && obj.AccessMethods[0].AccessID != "" {
				accessID = obj.AccessMethods[0].AccessID
			} else {
				// If no access ID, maybe direct URL?
				if obj.AccessMethods[0].AccessURL.URL != "" {
					return obj.AccessMethods[0].AccessURL.URL, nil
				}
				return "", fmt.Errorf("no suitable access method found for object %s", guid)
			}
		}
	}

	u, err := d.buildURL("ga4gh/drs/v1/objects", guid, "access", accessID)
	if err != nil {
		return "", err
	}

	var accessURL drs.AccessURL
	if err := d.doJSONRequest(ctx, http.MethodGet, u, nil, &accessURL); err != nil {
		return "", err
	}
	return accessURL.URL, nil
}

func (d *DrsBackend) GetObjectByHash(ctx context.Context, checksumType, checksum string) ([]drs.DRSObject, error) {
	// Query: GET /ga4gh/drs/v1/objects/checksum/<hash>
	// Note: checksumType is ignored here as per original implementation in LocalClient relying on checksum only in path.
	// Or should we use checksumType?
	u, err := d.buildURL("ga4gh/drs/v1/objects", "checksum", checksum)
	if err != nil {
		return nil, err
	}

	var objs []drs.DRSObject
	if err := d.doJSONRequest(ctx, http.MethodGet, u, nil, &objs); err != nil {
		return nil, err
	}

	return objs, nil
}

func (d *DrsBackend) BatchGetObjectsByHash(ctx context.Context, hashes []string) (map[string][]drs.DRSObject, error) {
	// Custom endpoint: POST /index/index/bulk/hashes
	// This path suggests it's mimicking Indexd API structure even if it's a DRS server
	u, err := d.buildURL("index/index/bulk/hashes")
	if err != nil {
		return nil, err
	}

	req := struct {
		Hashes []string `json:"hashes"`
	}{
		Hashes: hashes,
	}

	var list struct {
		Records []drs.DRSObject `json:"records"`
	}
	if err := d.doJSONRequest(ctx, http.MethodPost, u, req, &list); err != nil {
		return nil, err
	}

	result := make(map[string][]drs.DRSObject)
	for _, obj := range list.Records {
		if obj.Checksums.SHA256 != "" {
			result[obj.Checksums.SHA256] = append(result[obj.Checksums.SHA256], obj)
		}
	}
	return result, nil
}

func (d *DrsBackend) Register(ctx context.Context, obj *drs.DRSObject) (*drs.DRSObject, error) {
	u, err := d.buildURL("ga4gh/drs/v1/objects/register")
	if err != nil {
		return nil, err
	}

	req := drs.RegisterObjectsRequest{
		Candidates: []drs.DRSObjectCandidate{drs.ConvertToCandidate(obj)},
	}

	var registeredObjs []*drs.DRSObject
	if err := d.doJSONRequest(ctx, http.MethodPost, u, req, &registeredObjs); err != nil {
		return nil, err
	}

	if len(registeredObjs) == 0 {
		return nil, fmt.Errorf("server returned no registered objects")
	}

	return registeredObjs[0], nil
}

func (d *DrsBackend) BatchRegister(ctx context.Context, objs []*drs.DRSObject) ([]*drs.DRSObject, error) {
	u, err := d.buildURL("ga4gh/drs/v1/objects/register")
	if err != nil {
		return nil, err
	}

	var candidates []drs.DRSObjectCandidate
	for _, obj := range objs {
		candidates = append(candidates, drs.ConvertToCandidate(obj))
	}
	req := drs.RegisterObjectsRequest{
		Candidates: candidates,
	}

	var registeredObjs []*drs.DRSObject
	if err := d.doJSONRequest(ctx, http.MethodPost, u, req, &registeredObjs); err != nil {
		return nil, err
	}

	return registeredObjs, nil
}

func (d *DrsBackend) GetUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error) {
	// Hits the server's clean /data/upload/{file_id} endpoint
	u, err := d.buildURL("data/upload", guid)
	if err != nil {
		return "", err
	}
	// Add filename/bucket hints
	q := url.Values{}
	q.Set("file_name", filename)

	// Evaluate bucket from argument or struct
	effectiveBucket := bucket
	if effectiveBucket != "" {
		q.Set("bucket", effectiveBucket)
	}

	u += "?" + q.Encode()

	var res struct {
		URL string `json:"url"`
	}
	if err := d.doJSONRequest(ctx, http.MethodGet, u, nil, &res); err != nil {
		return "", err
	}
	return res.URL, nil
}

func (d *DrsBackend) Upload(ctx context.Context, url string, body io.Reader, size int64) error {
	rb := d.req.New(http.MethodPut, url).
		WithBody(body).
		WithSkipAuth(true) // S3 presigned URLs don't need our bearer token

	resp, err := d.req.Do(ctx, rb)
	if err != nil {
		return fmt.Errorf("upload to %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload to %s failed with status %d: %s", url, resp.StatusCode, string(bodyBytes))
	}

	return nil
}
