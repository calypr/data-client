package drs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/hash"
	"github.com/calypr/data-client/request"
)

type DrsClient struct {
	request.RequestInterface
	provider     endpointProvider
	logger       *slog.Logger
	projectId    string
	organization string
	bucketName   string
}

type endpointProvider interface {
	APIEndpoint() string
	AccessToken() string
}

type gen3Provider struct {
	cred *conf.Credential
}

func (p gen3Provider) APIEndpoint() string { return p.cred.APIEndpoint }
func (p gen3Provider) AccessToken() string { return p.cred.AccessToken }

type localProvider struct {
	endpoint string
}

func (p localProvider) APIEndpoint() string { return p.endpoint }
func (p localProvider) AccessToken() string { return "" }

// NewDrsClient creates a new DrsClient
func NewDrsClient(req request.RequestInterface, cred *conf.Credential, logger *slog.Logger) Client {
	return &DrsClient{
		RequestInterface: req,
		provider:         gen3Provider{cred: cred},
		logger:           logger,
	}
}

// NewLocalDrsClient creates a DRS client for local/non-Gen3 mode.
// It intentionally carries no bearer token.
func NewLocalDrsClient(req request.RequestInterface, endpoint string, logger *slog.Logger) Client {
	return &DrsClient{
		RequestInterface: req,
		provider:         localProvider{endpoint: endpoint},
		logger:           logger,
	}
}

func (c *DrsClient) apiEndpoint() string { return c.provider.APIEndpoint() }
func (c *DrsClient) token() string       { return c.provider.AccessToken() }

func (c *DrsClient) GetProjectId() string {
	return c.projectId
}

func (c *DrsClient) GetBucketName() string {
	return c.bucketName
}

func (c *DrsClient) GetOrganization() string {
	return c.organization
}

func (c *DrsClient) WithProject(projectId string) Client {
	c.projectId = projectId
	return c
}

func (c *DrsClient) WithOrganization(organization string) Client {
	c.organization = organization
	return c
}

func (c *DrsClient) WithBucket(bucketName string) Client {
	c.bucketName = bucketName
	return c
}

func (c *DrsClient) GetObject(ctx context.Context, id string) (*DRSObject, error) {
	url := fmt.Sprintf("%s/ga4gh/drs/v1/objects/%s", c.apiEndpoint(), id)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodGet,
		Url:    url,
		Token:  c.token(),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("object %s not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get object %s: %s (status: %d)", id, string(body), resp.StatusCode)
	}

	var out OutputObject
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return ConvertOutputObjectToDRSObject(&out), nil
}

func (c *DrsClient) GetObjectByHash(ctx context.Context, checksum *hash.Checksum) ([]DRSObject, error) {
	url := fmt.Sprintf("%s/index?hash=%s:%s", c.apiEndpoint(), string(checksum.Type), checksum.Checksum)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodGet,
		Url:    url,
		Headers: map[string]string{
			"Accept": "application/json",
		},
		Token: c.token(),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to query by hash %s:%s: %s (status: %d)", checksum.Type, checksum.Checksum, string(body), resp.StatusCode)
	}

	var records ListRecords
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, err
	}

	out := make([]DRSObject, 0, len(records.Records))
	for _, r := range records.Records {
		drsObj, err := r.ToDrsObject()
		if err != nil {
			return nil, err
		}
		out = append(out, *drsObj)
	}
	return out, nil
}

func (c *DrsClient) GetDownloadURL(ctx context.Context, id string, accessType string) (*AccessURL, error) {
	url := fmt.Sprintf("%s/ga4gh/drs/v1/objects/%s/access/%s", c.apiEndpoint(), id, accessType)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodGet,
		Url:    url,
		Token:  c.token(),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get download URL for %s: %s (status: %d)", id, string(body), resp.StatusCode)
	}

	var accessURL AccessURL
	if err := json.NewDecoder(resp.Body).Decode(&accessURL); err != nil {
		return nil, err
	}
	return &accessURL, nil
}

func (c *DrsClient) ListObjectsByProject(ctx context.Context, projectId string) (chan DRSObjectResult, error) {
	const PAGESIZE = 50

	resourcePath, err := ProjectToResource("", projectId)
	if err != nil {
		return nil, err
	}

	out := make(chan DRSObjectResult, PAGESIZE)

	go func() {
		defer close(out)
		pageNum := 0
		active := true

		for active {
			url := fmt.Sprintf("%s/index?authz=%s&limit=%d&page=%d",
				c.apiEndpoint(), resourcePath, PAGESIZE, pageNum)

			resp, err := c.Do(ctx, &request.RequestBuilder{
				Method: http.MethodGet,
				Url:    url,
				Headers: map[string]string{
					"Accept": "application/json",
				},
				Token: c.token(),
			})

			if err != nil {
				out <- DRSObjectResult{Error: err}
				break
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				out <- DRSObjectResult{Error: fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))}
				break
			}

			var page ListRecords
			err = json.NewDecoder(resp.Body).Decode(&page)
			resp.Body.Close()

			if err != nil {
				out <- DRSObjectResult{Error: err}
				break
			}

			if len(page.Records) == 0 {
				active = false
				break
			}

			for _, elem := range page.Records {
				drsObj, err := elem.ToDrsObject()
				if err != nil {
					out <- DRSObjectResult{Error: err}
					continue
				}
				out <- DRSObjectResult{Object: drsObj}
			}
			pageNum++
		}
	}()

	return out, nil
}

func (c *DrsClient) ListObjects(ctx context.Context) (chan DRSObjectResult, error) {
	url := fmt.Sprintf("%s/ga4gh/drs/v1/objects", c.apiEndpoint())
	const PAGESIZE = 50
	out := make(chan DRSObjectResult, 10)

	go func() {
		defer close(out)
		pageNum := 0
		active := true
		for active {
			fullURL := fmt.Sprintf("%s?limit=%d&page=%d", url, PAGESIZE, pageNum)
			resp, err := c.Do(ctx, &request.RequestBuilder{
				Method: http.MethodGet,
				Url:    fullURL,
				Token:  c.token(),
			})

			if err != nil {
				out <- DRSObjectResult{Error: err}
				return
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				out <- DRSObjectResult{Error: fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))}
				return
			}

			var page DRSPage
			err = json.NewDecoder(resp.Body).Decode(&page)
			resp.Body.Close()

			if err != nil {
				out <- DRSObjectResult{Error: err}
				return
			}

			if len(page.DRSObjects) == 0 {
				active = false
				break
			}

			for _, elem := range page.DRSObjects {
				elemCopy := elem
				out <- DRSObjectResult{Object: &elemCopy}
			}
			pageNum++
		}
	}()
	return out, nil
}

func (c *DrsClient) GetProjectSample(ctx context.Context, projectId string, limit int) ([]DRSObject, error) {
	if limit <= 0 {
		limit = 1
	}

	objChan, err := c.ListObjectsByProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	result := make([]DRSObject, 0, limit)
	for objResult := range objChan {
		if objResult.Error != nil {
			return nil, objResult.Error
		}
		result = append(result, *objResult.Object)

		if len(result) >= limit {
			go func() {
				for range objChan {
				}
			}()
			break
		}
	}
	return result, nil
}

func (c *DrsClient) RegisterRecord(ctx context.Context, record *DRSObject) (*DRSObject, error) {
	indexdRecord, err := InternalRecordFromDrsObject(record)
	if err != nil {
		return nil, fmt.Errorf("error converting DRS object to internal record: %v", err)
	}

	indexdObjForm := InternalRecordForm{
		InternalRecord: *indexdRecord,
		Form:           "object",
	}

	jsonBytes, err := json.Marshal(indexdObjForm)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/index", c.apiEndpoint())
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodPost,
		Url:    url,
		Body:   bytes.NewBuffer(jsonBytes),
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Token: c.token(),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		did := ""
		if indexdRecord.Did != nil {
			did = *indexdRecord.Did
		}
		return nil, fmt.Errorf("failed to register record %s: %s (status: %d)", did, string(body), resp.StatusCode)
	}

	return InternalRecordToDrsObject(indexdRecord)
}

func (c *DrsClient) RegisterRecords(ctx context.Context, records []*DRSObject) ([]*DRSObject, error) {
	if len(records) == 0 {
		return nil, nil
	}

	candidates := make([]DRSObjectCandidate, len(records))
	for i, r := range records {
		candidates[i] = ConvertToCandidate(r)
	}

	reqBody := RegisterObjectsRequest{
		Candidates: candidates,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/ga4gh/drs/v1/objects/register", c.apiEndpoint())
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodPost,
		Url:    url,
		Body:   bytes.NewBuffer(jsonBytes),
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Token: c.token(),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to register records: %s (status: %d)", string(body), resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading registered objects response: %v", err)
	}

	// Canonical shape from DRS register API.
	var wrapped struct {
		Objects []*DRSObject `json:"objects"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("unsupported response payload: %s", string(body))
	}
	if len(wrapped.Objects) == 0 {
		return nil, fmt.Errorf("register response did not include objects")
	}
	return wrapped.Objects, nil
}

func (c *DrsClient) UpdateRecord(ctx context.Context, updateInfo *DRSObject, did string) (*DRSObject, error) {
	// Get current revision from existing record
	record, err := c.getInternalRecordByDID(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve existing record for DID %s: %v", did, err)
	}

	// Build update payload starting with existing record values
	updatePayload := UpdateInputInfo{
		URLs:     record.Urls,
		FileName: record.FileName,
		Authz:    record.Authz,
	}

	// Apply updates from updateInfo
	if len(updateInfo.AccessMethods) > 0 {
		newURLs := make([]string, 0, len(updateInfo.AccessMethods))
		for _, a := range updateInfo.AccessMethods {
			if a.AccessUrl != nil {
				newURLs = append(newURLs, a.AccessUrl.Url)
			}
		}
		updatePayload.URLs = appendUnique(updatePayload.URLs, newURLs)

		authz := InternalAuthzFromDrsAccessMethods(updateInfo.AccessMethods)
		updatePayload.Authz = appendUnique(updatePayload.Authz, authz)
	}

	if updateInfo.Name != nil && *updateInfo.Name != "" {
		updatePayload.FileName = updateInfo.Name
	}

	if updateInfo.Version != nil && *updateInfo.Version != "" {
		updatePayload.Version = updateInfo.Version
	}

	if updateInfo.Description != nil && *updateInfo.Description != "" {
		if updatePayload.Metadata == nil {
			updatePayload.Metadata = make(map[string]any)
		}
		updatePayload.Metadata["description"] = *updateInfo.Description
	}

	jsonBytes, err := json.Marshal(updatePayload)
	if err != nil {
		return nil, fmt.Errorf("error marshaling indexd update payload: %v", err)
	}

	rev := ""
	if record.Rev != nil {
		rev = *record.Rev
	}
	url := fmt.Sprintf("%s/index/%s?rev=%s", c.apiEndpoint(), did, rev)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodPut,
		Url:    url,
		Body:   bytes.NewBuffer(jsonBytes),
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Token: c.token(),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to update record %s: %s (status: %d)", did, string(body), resp.StatusCode)
	}

	return c.GetObject(ctx, did)
}

func (c *DrsClient) DeleteRecord(ctx context.Context, did string) error {
	// First get the record to get the revision (rev)
	record, err := c.getInternalRecordByDID(ctx, did)
	if err != nil {
		return err
	}

	rev := ""
	if record.Rev != nil {
		rev = *record.Rev
	}
	url := fmt.Sprintf("%s/index/%s?rev=%s", c.apiEndpoint(), did, rev)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodDelete,
		Url:    url,
		Headers: map[string]string{
			"Accept": "application/json",
		},
		Token: c.token(),
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete record %s: %s (status: %d)", did, string(body), resp.StatusCode)
	}

	return nil
}

func (c *DrsClient) DeleteRecordsByProject(ctx context.Context, projectId string) error {
	recs, err := c.ListObjectsByProject(ctx, projectId)
	if err != nil {
		return err
	}

	ids := make([]string, 0, 128)
	seen := make(map[string]struct{})
	for rec := range recs {
		if rec.Error != nil {
			return rec.Error
		}

		if rec.Object == nil || rec.Object.Id == "" {
			continue
		}
		if _, ok := seen[rec.Object.Id]; ok {
			continue
		}
		seen[rec.Object.Id] = struct{}{}
		ids = append(ids, rec.Object.Id)
	}

	for _, id := range ids {
		err := c.DeleteRecord(ctx, id)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				continue
			}
			c.logger.Error(fmt.Sprintf("DeleteRecordsByProject Error for %s: %v", id, err))
			continue
		}
	}
	return nil
}

func (c *DrsClient) getInternalRecordByDID(ctx context.Context, did string) (*OutputInfo, error) {
	url := fmt.Sprintf("%s/index/%s", c.apiEndpoint(), did)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodGet,
		Url:    url,
		Token:  c.token(),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get internal record %s: %s (status: %d)", did, string(body), resp.StatusCode)
	}

	var info OutputInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *DrsClient) BatchGetObjectsByHash(ctx context.Context, hashes []string) (map[string][]DRSObject, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	reqBody := struct {
		Hashes []string `json:"hashes"`
	}{
		Hashes: hashes,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/index/bulk/hashes", c.apiEndpoint())
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodPost,
		Url:    url,
		Body:   bytes.NewBuffer(jsonBytes),
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Token: c.token(),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to bulk lookup hashes: %s (status: %d)", string(body), resp.StatusCode)
	}

	var list ListRecords
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	result := make(map[string][]DRSObject)
	for _, r := range list.Records {
		drsObj, err := r.ToDrsObject()
		if err != nil {
			continue
		}
		hInfo := hash.ConvertDrsChecksumsToHashInfo(drsObj.Checksums)
		if hInfo.SHA256 != "" {
			result[hInfo.SHA256] = append(result[hInfo.SHA256], *drsObj)
		}
	}

	return result, nil
}

func appendUnique(existing []string, toAdd []string) []string {
	seen := make(map[string]bool)
	for _, v := range existing {
		seen[v] = true
	}
	for _, v := range toAdd {
		if !seen[v] {
			existing = append(existing, v)
			seen[v] = true
		}
	}
	return existing
}

// BuildDrsObj matches git-drs behavior but moved to core
func (c *DrsClient) BuildDrsObj(fileName string, checksum string, size int64, drsId string) (*DRSObject, error) {
	return BuildDrsObj(fileName, checksum, size, drsId, c.GetBucketName(), c.GetOrganization(), c.GetProjectId())
}

// RegisterFile matches git-drs behavior but moved to core
func (c *DrsClient) RegisterFile(ctx context.Context, oid string, path string) (*DRSObject, error) {
	// Base implementation without LFS specifics
	return nil, fmt.Errorf("RegisterFile needs specific implementation (e.g. for LFS or cloud)")
}

func (c *DrsClient) DownloadFile(ctx context.Context, oid string, destPath string) error {
	return fmt.Errorf("DownloadFile implementation moved to high-level client")
}
