package indexd

import (
	"context"
	"fmt"
	"slices"

	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/s3utils"
)

// UpsertIndexdRecord creates or updates an indexd record with a new URL.
func (c *IndexdClient) UpsertIndexdRecord(ctx context.Context, url string, sha256 string, fileSize int64, projectId string) (*drs.DRSObject, error) {
	uuid := drs.DrsUUID(projectId, sha256)

	records, err := c.GetObjectByHash(ctx, "sha256", sha256)
	if err != nil {
		return nil, fmt.Errorf("error querying indexd server: %v", err)
	}

	var matchingRecord *drs.DRSObject
	for i := range records {
		if records[i].Id == uuid {
			matchingRecord = &records[i]
			break
		}
	}

	if matchingRecord != nil {
		existingURLs := IndexdURLFromDrsAccessURLs(matchingRecord.AccessMethods)
		if slices.Contains(existingURLs, url) {
			c.logger.Debug("Nothing to do: file already registered")
			return matchingRecord, nil
		}

		c.logger.Debug("updating existing record with new url")
		updatedRecord := drs.DRSObject{AccessMethods: []drs.AccessMethod{{AccessURL: drs.AccessURL{URL: url}}}}
		return c.UpdateRecord(ctx, &updatedRecord, matchingRecord.Id)
	}

	// If no record exists, create one
	c.logger.Debug("creating new record")
	_, key, err := s3utils.ParseS3URL(url)
	if err != nil {
		return nil, err
	}

	drsObj, err := drs.BuildDrsObj(key, sha256, fileSize, uuid, "placeholder-bucket", projectId)
	if err != nil {
		return nil, err
	}

	return c.RegisterRecord(ctx, drsObj)
}
