package s3utils

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"net/url"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/calypr/data-client/fence"
)

// ParseBlobURL parses a URL like s3://bucket/key and returns (bucket, key, error).
func ParseBlobURL(blobURL string) (string, string, error) {
	u, err := url.Parse(blobURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid blob URL %s: %w", blobURL, err)
	}
	if u.Scheme == "" {
		return "", "", fmt.Errorf("URL requires a scheme prefix (e.g. s3://, gs://): %s", blobURL)
	}

	bucket := u.Host
	if u.Scheme == "file" {
		bucket = "file:///"
	}
	
	key := strings.TrimPrefix(u.Path, "/")
	if key == "" {
		return "", "", fmt.Errorf("invalid blob URL (missing key/path): %s", blobURL)
	}

	return bucket, key, nil
}

// ValidateInputs checks if the Blob URL and SHA256 hash are valid.
func ValidateInputs(s3URL, sha256 string) error {
	if s3URL == "" {
		return fmt.Errorf("Blob URL is required")
	}
	if sha256 == "" {
		return fmt.Errorf("SHA256 hash is required")
	}
	u, err := url.Parse(s3URL)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("invalid Blob URL: must have a scheme (like s3://, gs://)")
	}
	if len(sha256) != 64 {
		return fmt.Errorf("invalid SHA256 hash: must be 64 characters")
	}
	return nil
}

// FetchS3MetadataWithBucketDetails fetches S3 metadata (size and modified date) for a given S3 URL.
// FetchS3MetadataWithBucketDetails fetches metadata using generic go-cloud capabilities.
// This makes it compatible with multiple cloud providers instead of a bare-bones specific setup.
func FetchS3MetadataWithBucketDetails(
	ctx context.Context,
	s3URL string,
	awsAccessKey string,
	awsSecretKey string,
	region string,
	endpoint string,
	bucketDetails *fence.S3Bucket,
	s3Client *s3.Client, // kept for backward compatibility signature, though it is no longer strictly required for basic HEAD.
	logger *slog.Logger,
) (int64, string, error) {
	u, err := url.Parse(s3URL)
	if err != nil {
		return 0, "", fmt.Errorf("failed to parse url %s: %w", s3URL, err)
	}

	bucketURL := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	key := strings.TrimPrefix(u.Path, "/")

	// Optionally pass credentials logic. By default go-cloud checks environment.
	// For AWS, you could override credentials, but typically users want standard config loading
	// which go-cloud openers handle out of the box (e.g. AWS_PROFILE, AWS_REGION, AWS_ACCESS_KEY_ID).

	bucket, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		return 0, "", fmt.Errorf("failed to open bucket via go-cloud string %s: %w", bucketURL, err)
	}
	defer bucket.Close()

	attrs, err := bucket.Attributes(ctx, key)
	if err != nil {
		return 0, "", fmt.Errorf("failed to get attributes for %s: %w", key, err)
	}

	lastMod := ""
	if !attrs.ModTime.IsZero() {
		lastMod = attrs.ModTime.Format(time.RFC3339)
	}

	return attrs.Size, lastMod, nil
}

type S3Meta struct {
	Size         int64
	LastModified string
}
