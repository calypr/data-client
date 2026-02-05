package s3utils

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/calypr/data-client/fence"
)

// ParseS3URL parses a URL like s3://bucket/key and returns (bucket, key, error).
func ParseS3URL(s3url string) (string, string, error) {
	s3Prefix := "s3://"
	if !strings.HasPrefix(s3url, s3Prefix) {
		return "", "", fmt.Errorf("S3 URL requires prefix 's3://': %s", s3url)
	}
	trimmed := strings.TrimPrefix(s3url, s3Prefix)
	slashIndex := strings.Index(trimmed, "/")
	if slashIndex == -1 || slashIndex == len(trimmed)-1 {
		return "", "", fmt.Errorf("invalid S3 file URL: %s", s3url)
	}
	return trimmed[:slashIndex], trimmed[slashIndex+1:], nil
}

// ValidateInputs checks if S3 URL and SHA256 hash are valid.
func ValidateInputs(s3URL, sha256 string) error {
	if s3URL == "" {
		return fmt.Errorf("S3 URL is required")
	}
	if sha256 == "" {
		return fmt.Errorf("SHA256 hash is required")
	}
	if !strings.HasPrefix(s3URL, "s3://") {
		return fmt.Errorf("invalid S3 URL: must start with s3://")
	}
	if len(sha256) != 64 {
		return fmt.Errorf("invalid SHA256 hash: must be 64 characters")
	}
	return nil
}

// FetchS3MetadataWithBucketDetails fetches S3 metadata (size and modified date) for a given S3 URL.
func FetchS3MetadataWithBucketDetails(
	ctx context.Context,
	s3URL string,
	awsAccessKey string,
	awsSecretKey string,
	region string,
	endpoint string,
	bucketDetails *fence.S3Bucket,
	s3Client *s3.Client,
	logger *slog.Logger,
) (int64, string, error) {
	bucket, key, err := ParseS3URL(s3URL)
	if err != nil {
		return 0, "", err
	}

	if s3Client == nil {
		var configOptions []func(*awsConfig.LoadOptions) error
		if awsAccessKey != "" && awsSecretKey != "" {
			configOptions = append(configOptions,
				awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, "")),
			)
		}

		regionToUse := ""
		if region != "" {
			regionToUse = region
		} else if bucketDetails != nil && bucketDetails.Region != "" {
			regionToUse = bucketDetails.Region
		}
		if regionToUse != "" {
			configOptions = append(configOptions, awsConfig.WithRegion(regionToUse))
		}

		cfg, err := awsConfig.LoadDefaultConfig(ctx, configOptions...)
		if err != nil {
			return 0, "", fmt.Errorf("unable to load AWS SDK config: %w", err)
		}

		endpointToUse := ""
		if endpoint != "" {
			endpointToUse = endpoint
		} else if bucketDetails != nil && bucketDetails.EndpointURL != "" {
			endpointToUse = bucketDetails.EndpointURL
		}

		s3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			if endpointToUse != "" {
				o.BaseEndpoint = aws.String(endpointToUse)
			}
			o.UsePathStyle = true
		})
	}

	input := &s3.HeadObjectInput{
		Bucket: &bucket,
		Key:    aws.String(key),
	}

	resp, err := s3Client.HeadObject(ctx, input)
	if err != nil {
		return 0, "", fmt.Errorf("failed to head object: %w", err)
	}

	var contentLength int64
	if resp.ContentLength != nil {
		contentLength = *resp.ContentLength
	}

	var lastModified string
	if resp.LastModified != nil {
		lastModified = resp.LastModified.Format(time.RFC3339)
	}

	return contentLength, lastModified, nil
}

type S3Meta struct {
	Size         int64
	LastModified string
}
