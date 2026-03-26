package drs

import (
	"context"
	"fmt"
	"slices"

	"github.com/calypr/data-client/hash"
)

// UpsertRecord creates or updates a record with a new URL.
func (c *DrsClient) UpsertRecord(ctx context.Context, url string, sha256 string, fileSize int64, projectId string) (*DRSObject, error) {
	sha256 = NormalizeOid(sha256)
	
	// Query current state
	records, err := c.GetObjectByHash(ctx, &hash.Checksum{Type: hash.ChecksumTypeSHA256, Checksum: sha256})
	if err != nil {
		return nil, fmt.Errorf("error querying DRS server: %v", err)
	}

	var matchingRecord *DRSObject
	for i := range records {
		// Match by checksum content identity
		if hash.ConvertDrsChecksumsToHashInfo(records[i].Checksums).SHA256 == sha256 {
			matchingRecord = &records[i]
			break
		}
	}

	if matchingRecord != nil {
		existingURLs := InternalURLFromDrsAccessURLs(matchingRecord.AccessMethods)
		if slices.Contains(existingURLs, url) {
			return matchingRecord, nil
		}

		c.logger.Debug("updating existing record with new url")
		updatedRecord := DRSObject{AccessMethods: []AccessMethod{{AccessUrl: &AccessURL{Url: url}}}}
		return c.UpdateRecord(ctx, &updatedRecord, matchingRecord.Id)
	}

	// If no record exists, create one
	c.logger.Debug("creating new record")
	uuid := GenerateDrsID(projectId, sha256)
	
	// Use simplified BuildDrsObj (helper in same package)
	drsObj, err := BuildDrsObj("", sha256, fileSize, uuid, c.GetBucketName(), c.GetOrganization(), projectId)
	if err != nil {
		return nil, err
	}

	return c.RegisterRecord(ctx, drsObj)
}

// Internal methods to support specialized behaviors from git-drs
// (These can be overridden or extended)

func (c *DrsClient) AddURL(ctx context.Context, blobURL, sha256 string, opts ...AddURLOption) (*DRSObject, error) {
	// Simple wrapper for UpsertRecord for now, but allows for more complex logic if needed
	// In a real implementation, this would handle cloud inspection too.
	return c.UpsertRecord(ctx, blobURL, sha256, 0, c.GetProjectId())
}
