package indexd

import (
	"context"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/calypr/data-client/fence"
	"github.com/calypr/data-client/indexd/drs"
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
	_, key, err := ParseS3URL(url)
	if err != nil {
		return nil, err
	}

	drsObj, err := drs.BuildDrsObj(key, sha256, fileSize, uuid, "placeholder-bucket", projectId)
	if err != nil {
		return nil, err
	}

	return c.RegisterRecord(ctx, drsObj)
}

// AddURL implements the AddURL logic ported from git-drs.
func (c *IndexdClient) AddURL(
	ctx context.Context,
	fClient fence.FenceInterface,
	s3URL string,
	sha256 string,
	awsAccessKey string,
	awsSecretKey string,
	region string,
	endpoint string,
	s3Client *s3.Client,
) (S3Meta, error) {
	if err := ValidateInputs(s3URL, sha256); err != nil {
		return S3Meta{}, err
	}

	bucket, _, err := ParseS3URL(s3URL)
	if err != nil {
		return S3Meta{}, err
	}

	var bucketDetails *fence.S3Bucket
	if fClient != nil {
		bucketDetails, err = fClient.GetBucketDetails(ctx, bucket)
		if err != nil {
			c.logger.Debug(fmt.Sprintf("Warning: unable to get bucket details from Gen3: %v", err))
		}
	}

	size, modifiedDate, err := FetchS3MetadataWithBucketDetails(
		ctx, s3URL, awsAccessKey, awsSecretKey, region, endpoint, bucketDetails, s3Client, c.logger,
	)
	if err != nil {
		return S3Meta{}, fmt.Errorf("failed to fetch S3 metadata: %w", err)
	}

	// This part needs project ID. In git-drs it was in the client config.
	projectId := "unknown-project"
	// ... (logic to get project ID)

	_, err = c.UpsertIndexdRecord(ctx, s3URL, sha256, size, projectId)
	if err != nil {
		return S3Meta{}, fmt.Errorf("failed to upsert indexd record: %w", err)
	}

	return S3Meta{
		Size:         size,
		LastModified: modifiedDate,
	}, nil
}
