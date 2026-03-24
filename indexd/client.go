package indexd

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
	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/request"
)

//go:generate mockgen -destination=../mocks/mock_indexd.go -package=mocks github.com/calypr/data-client/indexd IndexdInterface

// IndexdInterface defines the interface for Indexd client
type IndexdInterface interface {
	request.RequestInterface

	GetObject(ctx context.Context, id string) (*drs.DRSObject, error)
	RegisterIndexdRecord(ctx context.Context, indexdObj *IndexdRecord) (*drs.DRSObject, error)
	DeleteIndexdRecord(ctx context.Context, did string) error
	GetObjectByHash(ctx context.Context, hashType, hashValue string) ([]drs.DRSObject, error)
	GetDownloadURL(ctx context.Context, did string, accessType string) (*drs.AccessURL, error)
	ListObjectsByProject(ctx context.Context, projectId string) (chan drs.DRSObjectResult, error)
	UpdateRecord(ctx context.Context, updateInfo *drs.DRSObject, did string) (*drs.DRSObject, error)

	ListObjects(ctx context.Context) (chan drs.DRSObjectResult, error)
	GetProjectSample(ctx context.Context, projectId string, limit int) ([]drs.DRSObject, error)
	DeleteRecordsByProject(ctx context.Context, projectId string) error
	DeleteRecordByHash(ctx context.Context, hashValue string, projectId string) error
	RegisterRecord(ctx context.Context, record *drs.DRSObject) (*drs.DRSObject, error)
	RegisterRecords(ctx context.Context, records []*drs.DRSObject) ([]*drs.DRSObject, error)
	UpsertIndexdRecord(ctx context.Context, url string, sha256 string, fileSize int64, projectId string) (*drs.DRSObject, error)

	BatchGetObjectsByHash(ctx context.Context, hashes []string) (map[string][]drs.DRSObject, error)
}

// IndexdClient implements IndexdInterface
type IndexdClient struct {
	request.RequestInterface
	cred   *conf.Credential
	logger *slog.Logger
}

// NewIndexdClient creates a new IndexdClient
func NewIndexdClient(req request.RequestInterface, cred *conf.Credential, logger *slog.Logger) IndexdInterface {
	return &IndexdClient{
		RequestInterface: req,
		cred:             cred,
		logger:           logger,
	}
}

func (c *IndexdClient) GetObject(ctx context.Context, id string) (*drs.DRSObject, error) {
	url := fmt.Sprintf("%s/ga4gh/drs/v1/objects/%s", c.cred.APIEndpoint, id)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodGet,
		Url:    url,
		Token:  c.cred.AccessToken,
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

func (c *IndexdClient) RegisterIndexdRecord(ctx context.Context, indexdObj *IndexdRecord) (*drs.DRSObject, error) {
	indexdObjForm := IndexdRecordForm{
		IndexdRecord: *indexdObj,
		Form:         "object",
	}

	jsonBytes, err := json.Marshal(indexdObjForm)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/index", c.cred.APIEndpoint)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodPost,
		Url:    url,
		Body:   bytes.NewBuffer(jsonBytes),
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Token: c.cred.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to register record %s: %s (status: %d)", indexdObj.Did, string(body), resp.StatusCode)
	}

	return IndexdRecordToDrsObject(indexdObj)
}

func (c *IndexdClient) DeleteIndexdRecord(ctx context.Context, did string) error {
	// First get the record to get the revision (rev)
	record, err := c.getIndexdRecordByDID(ctx, did)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/index/%s?rev=%s", c.cred.APIEndpoint, did, record.Rev)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodDelete,
		Url:    url,
		Headers: map[string]string{
			"Accept": "application/json",
		},
		Token: c.cred.AccessToken,
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

func (c *IndexdClient) getIndexdRecordByDID(ctx context.Context, did string) (*OutputInfo, error) {
	url := fmt.Sprintf("%s/index/%s", c.cred.APIEndpoint, did)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodGet,
		Url:    url,
		Token:  c.cred.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get indexd record %s: %s (status: %d)", did, string(body), resp.StatusCode)
	}

	var info OutputInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *IndexdClient) GetObjectByHash(ctx context.Context, hashType, hashValue string) ([]drs.DRSObject, error) {
	url := fmt.Sprintf("%s/index?hash=%s:%s", c.cred.APIEndpoint, hashType, hashValue)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodGet,
		Url:    url,
		Headers: map[string]string{
			"Accept": "application/json",
		},
		Token: c.cred.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to query by hash %s:%s: %s (status: %d)", hashType, hashValue, string(body), resp.StatusCode)
	}

	var records ListRecords
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, err
	}

	out := make([]drs.DRSObject, 0, len(records.Records))
	for _, r := range records.Records {
		drsObj, err := IndexdRecordToDrsObject(r.ToIndexdRecord())
		if err != nil {
			return nil, err
		}
		out = append(out, *drsObj)
	}
	return out, nil
}

func (c *IndexdClient) GetDownloadURL(ctx context.Context, did string, accessType string) (*drs.AccessURL, error) {
	url := fmt.Sprintf("%s/ga4gh/drs/v1/objects/%s/access/%s", c.cred.APIEndpoint, did, accessType)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodGet,
		Url:    url,
		Token:  c.cred.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get download URL for %s: %s (status: %d)", did, string(body), resp.StatusCode)
	}

	var accessURL drs.AccessURL
	if err := json.NewDecoder(resp.Body).Decode(&accessURL); err != nil {
		return nil, err
	}
	return &accessURL, nil
}

func (c *IndexdClient) ListObjectsByProject(ctx context.Context, projectId string) (chan drs.DRSObjectResult, error) {
	const PAGESIZE = 50

	resourcePath, err := drs.ProjectToResource("", projectId)
	if err != nil {
		return nil, err
	}

	out := make(chan drs.DRSObjectResult, PAGESIZE)

	go func() {
		defer close(out)
		pageNum := 0
		active := true

		for active {
			url := fmt.Sprintf("%s/index?authz=%s&limit=%d&page=%d",
				c.cred.APIEndpoint, resourcePath, PAGESIZE, pageNum)

			resp, err := c.Do(ctx, &request.RequestBuilder{
				Method: http.MethodGet,
				Url:    url,
				Headers: map[string]string{
					"Accept": "application/json",
				},
				Token: c.cred.AccessToken,
			})

			if err != nil {
				out <- drs.DRSObjectResult{Error: err}
				break
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				out <- drs.DRSObjectResult{Error: fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))}
				break
			}

			var page ListRecords
			err = json.NewDecoder(resp.Body).Decode(&page)
			resp.Body.Close()

			if err != nil {
				out <- drs.DRSObjectResult{Error: err}
				break
			}

			if len(page.Records) == 0 {
				active = false
				break
			}

			for _, elem := range page.Records {
				drsObj, err := elem.ToIndexdRecord().ToDrsObject()
				if err != nil {
					out <- drs.DRSObjectResult{Error: err}
					continue
				}
				out <- drs.DRSObjectResult{Object: drsObj}
			}
			pageNum++
		}
	}()

	return out, nil
}

func (c *IndexdClient) UpdateRecord(ctx context.Context, updateInfo *drs.DRSObject, did string) (*drs.DRSObject, error) {
	// Get current revision from existing record
	record, err := c.getIndexdRecordByDID(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve existing record for DID %s: %v", did, err)
	}

	// Build update payload starting with existing record values
	updatePayload := UpdateInputInfo{
		URLs:     record.URLs,
		FileName: record.FileName,
		Version:  record.Version,
		Authz:    record.Authz,
		ACL:      record.ACL,
		Metadata: record.Metadata,
	}

	// Apply updates from updateInfo
	if len(updateInfo.AccessMethods) > 0 {
		newURLs := make([]string, 0, len(updateInfo.AccessMethods))
		for _, a := range updateInfo.AccessMethods {
			newURLs = append(newURLs, a.AccessURL.URL)
		}
		updatePayload.URLs = appendUnique(updatePayload.URLs, newURLs)

		authz := IndexdAuthzFromDrsAccessMethods(updateInfo.AccessMethods)
		updatePayload.Authz = appendUnique(updatePayload.Authz, authz)
	}

	if updateInfo.Name != "" {
		updatePayload.FileName = updateInfo.Name
	}

	if updateInfo.Version != "" {
		updatePayload.Version = updateInfo.Version
	}

	if updateInfo.Description != "" {
		if updatePayload.Metadata == nil {
			updatePayload.Metadata = make(map[string]any)
		}
		updatePayload.Metadata["description"] = updateInfo.Description
	}

	jsonBytes, err := json.Marshal(updatePayload)
	if err != nil {
		return nil, fmt.Errorf("error marshaling indexd update payload: %v", err)
	}

	url := fmt.Sprintf("%s/index/%s?rev=%s", c.cred.APIEndpoint, did, record.Rev)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodPut,
		Url:    url,
		Body:   bytes.NewBuffer(jsonBytes),
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Token: c.cred.AccessToken,
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

func (c *IndexdClient) ListObjects(ctx context.Context) (chan drs.DRSObjectResult, error) {
	url := fmt.Sprintf("%s/ga4gh/drs/v1/objects", c.cred.APIEndpoint)
	const PAGESIZE = 50
	out := make(chan drs.DRSObjectResult, 10)

	go func() {
		defer close(out)
		pageNum := 0
		active := true
		for active {
			fullURL := fmt.Sprintf("%s?limit=%d&page=%d", url, PAGESIZE, pageNum)
			resp, err := c.Do(ctx, &request.RequestBuilder{
				Method: http.MethodGet,
				Url:    fullURL,
				Token:  c.cred.AccessToken,
			})

			if err != nil {
				out <- drs.DRSObjectResult{Error: err}
				return
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				out <- drs.DRSObjectResult{Error: fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))}
				return
			}

			var page drs.DRSPage
			err = json.NewDecoder(resp.Body).Decode(&page)
			resp.Body.Close()

			if err != nil {
				out <- drs.DRSObjectResult{Error: err}
				return
			}

			if len(page.DRSObjects) == 0 {
				active = false
				break
			}

			for _, elem := range page.DRSObjects {
				out <- drs.DRSObjectResult{Object: &elem}
			}
			pageNum++
		}
	}()
	return out, nil
}

func (c *IndexdClient) GetProjectSample(ctx context.Context, projectId string, limit int) ([]drs.DRSObject, error) {
	if limit <= 0 {
		limit = 1
	}

	objChan, err := c.ListObjectsByProject(ctx, projectId)
	if err != nil {
		return nil, err
	}

	result := make([]drs.DRSObject, 0, limit)
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

func (c *IndexdClient) DeleteRecordsByProject(ctx context.Context, projectId string) error {
	recs, err := c.ListObjectsByProject(ctx, projectId)
	if err != nil {
		return err
	}

	// Snapshot and dedupe IDs first so pagination isn't affected by deletes-in-flight.
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
		err := c.DeleteIndexdRecord(ctx, id)
		if err != nil {
			// Project-wide cleanup should be idempotent; stale/deleted IDs are expected.
			if isNotFoundErr(err) {
				c.logger.Info(fmt.Sprintf("DeleteRecordsByProject: record already absent %s", id))
				continue
			}
			c.logger.Error(fmt.Sprintf("DeleteRecordsByProject Error for %s: %v", id, err))
			continue
		}
	}
	return nil
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status: 404") ||
		strings.Contains(msg, "status=404") ||
		strings.Contains(msg, "Object not found") ||
		strings.Contains(msg, "not found")
}

func (c *IndexdClient) DeleteRecordByHash(ctx context.Context, hashValue string, projectId string) error {
	records, err := c.GetObjectByHash(ctx, "sha256", hashValue)
	if err != nil {
		return fmt.Errorf("error getting records for hash %s: %v", hashValue, err)
	}
	if len(records) == 0 {
		return fmt.Errorf("no records found for hash %s", hashValue)
	}

	matchingRecord, err := drs.FindMatchingRecord(records, "", projectId)
	if err != nil {
		return fmt.Errorf("error finding matching record for project %s: %v", projectId, err)
	}
	if matchingRecord == nil {
		return fmt.Errorf("no matching record found for project %s", projectId)
	}

	return c.DeleteIndexdRecord(ctx, matchingRecord.Id)
}

func (c *IndexdClient) RegisterRecord(ctx context.Context, record *drs.DRSObject) (*drs.DRSObject, error) {
	indexdRecord, err := IndexdRecordFromDrsObject(record)
	if err != nil {
		return nil, fmt.Errorf("error converting DRS object to indexd record: %v", err)
	}

	return c.RegisterIndexdRecord(ctx, indexdRecord)
}

func (c *IndexdClient) RegisterRecords(ctx context.Context, records []*drs.DRSObject) ([]*drs.DRSObject, error) {
	if len(records) == 0 {
		return nil, nil
	}

	candidates := make([]drs.DRSObjectCandidate, len(records))
	for i, r := range records {
		candidates[i] = drs.ConvertToCandidate(r)
	}

	reqBody := drs.RegisterObjectsRequest{
		Candidates: candidates,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/ga4gh/drs/v1/objects/register", c.cred.APIEndpoint)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodPost,
		Url:    url,
		Body:   bytes.NewBuffer(jsonBytes),
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Token: c.cred.AccessToken,
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

	registered, err := decodeRegisteredObjects(body)
	if err != nil {
		return nil, fmt.Errorf("error decoding registered objects: %v", err)
	}
	return registered, nil
}

func decodeRegisteredObjects(body []byte) ([]*drs.DRSObject, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty response body")
	}

	// Canonical shape from DRS register API.
	var wrapped struct {
		Objects []*drs.DRSObject `json:"objects"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err != nil {
		return nil, fmt.Errorf("unsupported response payload: %s", string(trimmed))
	}
	if len(wrapped.Objects) == 0 {
		return nil, fmt.Errorf("register response did not include objects")
	}
	return wrapped.Objects, nil
}

func (c *IndexdClient) BatchGetObjectsByHash(ctx context.Context, hashes []string) (map[string][]drs.DRSObject, error) {
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

	url := fmt.Sprintf("%s/index/bulk/hashes", c.cred.APIEndpoint)
	resp, err := c.Do(ctx, &request.RequestBuilder{
		Method: http.MethodPost,
		Url:    url,
		Body:   bytes.NewBuffer(jsonBytes),
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Token: c.cred.AccessToken,
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

	result := make(map[string][]drs.DRSObject)
	for _, r := range list.Records {
		drsObj, err := r.ToIndexdRecord().ToDrsObject()
		if err != nil {
			continue
		}
		// Group by hash. We use the SHA256 as the key.
		if drsObj.Checksums.SHA256 != "" {
			result[drsObj.Checksums.SHA256] = append(result[drsObj.Checksums.SHA256], *drsObj)
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
