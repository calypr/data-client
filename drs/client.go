package drs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/hash"
	"github.com/calypr/data-client/request"
	syclient "github.com/calypr/syfon/client"
)

type DrsClient struct {
	request.RequestInterface
	provider     endpointProvider
	syfon        *syclient.Client
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

// NewDrsClient creates a new DrsClient.
func NewDrsClient(req request.RequestInterface, cred *conf.Credential, logger *slog.Logger) Client {
	provider := gen3Provider{cred: cred}
	return &DrsClient{
		RequestInterface: req,
		provider:         provider,
		syfon:            buildSyfonClient(req, provider.APIEndpoint(), provider.AccessToken()),
		logger:           logger,
	}
}

// NewLocalDrsClient creates a DRS client for local/non-Gen3 mode.
// It intentionally carries no bearer token.
func NewLocalDrsClient(req request.RequestInterface, endpoint string, logger *slog.Logger) Client {
	provider := localProvider{endpoint: endpoint}
	return &DrsClient{
		RequestInterface: req,
		provider:         provider,
		syfon:            buildSyfonClient(req, provider.APIEndpoint(), ""),
		logger:           logger,
	}
}

func buildSyfonClient(req request.RequestInterface, endpoint, token string) *syclient.Client {
	opts := make([]syclient.Option, 0, 2)
	if strings.TrimSpace(token) != "" {
		opts = append(opts, syclient.WithBearerToken(token))
	}
	if r, ok := req.(*request.Request); ok && r.RetryClient != nil {
		opts = append(opts, syclient.WithHTTPClient(r.RetryClient.StandardClient()))
	}
	return syclient.New(endpoint, opts...)
}

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
	obj, err := c.syfon.DRS().GetObject(ctx, id)
	if err != nil {
		return nil, err
	}
	return &obj, nil
}

func (c *DrsClient) GetObjectByHash(ctx context.Context, checksum *hash.Checksum) ([]DRSObject, error) {
	if checksum == nil {
		return nil, fmt.Errorf("checksum is required")
	}
	resp, err := c.syfon.Index().List(ctx, syclient.ListRecordsOptions{
		Hash: fmt.Sprintf("%s:%s", string(checksum.Type), checksum.Checksum),
	})
	if err != nil {
		return nil, err
	}

	out := make([]DRSObject, 0, len(resp.Records))
	for _, rec := range resp.Records {
		drsObj, err := syfonInternalRecordToDRSObject(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, *drsObj)
	}
	return out, nil
}

func (c *DrsClient) GetDownloadURL(ctx context.Context, id string, accessType string) (*AccessURL, error) {
	access, err := c.syfon.DRS().GetAccessURL(ctx, id, accessType)
	if err != nil {
		return nil, err
	}
	return &AccessURL{Url: access.Url}, nil
}

func (c *DrsClient) ListObjectsByProject(ctx context.Context, projectId string) (chan DRSObjectResult, error) {
	resourcePath, err := ProjectToResource(c.organization, projectId)
	if err != nil {
		return nil, err
	}

	resp, err := c.syfon.Index().List(ctx, syclient.ListRecordsOptions{Authz: resourcePath})
	if err != nil {
		return nil, err
	}

	out := make(chan DRSObjectResult, len(resp.Records))
	go func() {
		defer close(out)
		for _, elem := range resp.Records {
			drsObj, err := syfonInternalRecordToDRSObject(elem)
			if err != nil {
				out <- DRSObjectResult{Error: err}
				continue
			}
			out <- DRSObjectResult{Object: drsObj}
		}
	}()
	return out, nil
}

func (c *DrsClient) ListObjects(ctx context.Context) (chan DRSObjectResult, error) {
	const pageSize = 50
	out := make(chan DRSObjectResult, pageSize)

	go func() {
		defer close(out)
		for page := 0; ; page++ {
			resp, err := c.syfon.DRS().ListObjects(ctx, pageSize, page)
			if err != nil {
				out <- DRSObjectResult{Error: err}
				return
			}
			if len(resp.DrsObjects) == 0 {
				return
			}
			for _, elem := range resp.DrsObjects {
				obj := elem
				out <- DRSObjectResult{Object: &obj}
			}
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
	internalRecord, err := drsObjectToSyfonInternalRecord(record)
	if err != nil {
		return nil, err
	}
	created, err := c.syfon.Index().Create(ctx, internalRecord)
	if err != nil {
		return nil, err
	}
	return syfonInternalRecordToDRSObject(created)
}

func (c *DrsClient) RegisterRecords(ctx context.Context, records []*DRSObject) ([]*DRSObject, error) {
	if len(records) == 0 {
		return nil, nil
	}

	candidates := make([]syclient.DRSObjectCandidate, len(records))
	for i, r := range records {
		candidates[i] = ConvertToCandidate(r)
	}

	resp, err := c.syfon.DRS().RegisterObjects(ctx, syclient.RegisterObjectsRequest{Candidates: candidates})
	if err != nil {
		return nil, err
	}
	if len(resp.Objects) == 0 {
		return nil, fmt.Errorf("register response did not include objects")
	}

	out := make([]*DRSObject, 0, len(resp.Objects))
	for _, obj := range resp.Objects {
		o := obj
		out = append(out, &o)
	}
	return out, nil
}

func (c *DrsClient) UpdateRecord(ctx context.Context, updateInfo *DRSObject, did string) (*DRSObject, error) {
	existing, err := c.syfon.Index().Get(ctx, did)
	if err != nil {
		return nil, err
	}

	updated := existing
	updated.SetDid(did)
		if len(updateInfo.AccessMethods) > 0 {
			newURLs := make([]string, 0, len(updateInfo.AccessMethods))
			for _, a := range updateInfo.AccessMethods {
				if a.AccessUrl.Url == "" {
					continue
				}
				newURLs = append(newURLs, a.AccessUrl.Url)
			}
			updated.SetUrls(appendUnique(updated.GetUrls(), newURLs))

		authz := InternalAuthzFromDrsAccessMethods(updateInfo.AccessMethods)
		updated.SetAuthz(appendUnique(updated.GetAuthz(), authz))
	}

	if updateInfo.Name != "" {
		updated.SetFileName(updateInfo.Name)
	}
	if updateInfo.Size > 0 {
		updated.SetSize(updateInfo.Size)
	}
	if len(updateInfo.Checksums) > 0 {
		updated.SetHashes(hash.ConvertDrsChecksumsToMap(updateInfo.Checksums))
	}

	res, err := c.syfon.Index().Update(ctx, did, updated)
	if err != nil {
		return nil, err
	}
	return syfonInternalRecordToDRSObject(res)
}

func (c *DrsClient) DeleteRecord(ctx context.Context, did string) error {
	return c.syfon.Index().Delete(ctx, did)
}

func (c *DrsClient) DeleteRecordsByProject(ctx context.Context, projectId string) error {
	resourcePath, err := ProjectToResource(c.organization, projectId)
	if err != nil {
		return err
	}
	_, err = c.syfon.Index().DeleteByQuery(ctx, syclient.DeleteByQueryOptions{Authz: resourcePath})
	return err
}

func (c *DrsClient) BatchGetObjectsByHash(ctx context.Context, hashes []string) (map[string][]DRSObject, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	resp, err := c.syfon.Index().BulkHashes(ctx, syclient.BulkHashesRequest{Hashes: hashes})
	if err != nil {
		return nil, err
	}

	result := make(map[string][]DRSObject)
	for _, rec := range resp.Records {
		drsObj, err := syfonInternalRecordToDRSObject(rec)
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

// BuildDrsObj matches git-drs behavior but moved to core.
func (c *DrsClient) BuildDrsObj(fileName string, checksum string, size int64, drsId string) (*DRSObject, error) {
	return BuildDrsObj(fileName, checksum, size, drsId, c.GetBucketName(), c.GetOrganization(), c.GetProjectId())
}

// RegisterFile matches git-drs behavior but moved to core.
func (c *DrsClient) RegisterFile(ctx context.Context, oid string, path string) (*DRSObject, error) {
	// Base implementation without LFS specifics.
	return nil, fmt.Errorf("RegisterFile needs specific implementation (e.g. for LFS or cloud)")
}

func (c *DrsClient) DownloadFile(ctx context.Context, oid string, destPath string) error {
	return fmt.Errorf("DownloadFile implementation moved to high-level client")
}
