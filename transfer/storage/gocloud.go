package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
)

// Bucket abstracts cross-cloud object operations used by transfer paths.
type Bucket interface {
	Upload(ctx context.Context, key string, body io.Reader) error
	Download(ctx context.Context, key string) ([]byte, error)
	SignedDownloadURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	SignedUploadURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	Close() error
}

type GoCloudBucket struct {
	b *blob.Bucket
}

// Open opens a go-cloud bucket URL, e.g.:
// s3://bucket, gs://bucket, azblob://container
func Open(ctx context.Context, bucketURL string) (Bucket, error) {
	b, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		return nil, err
	}
	return &GoCloudBucket{b: b}, nil
}

func (g *GoCloudBucket) Upload(ctx context.Context, key string, body io.Reader) error {
	w, err := g.b.NewWriter(ctx, key, nil)
	if err != nil {
		return err
	}
	if _, err = io.Copy(w, body); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func (g *GoCloudBucket) Download(ctx context.Context, key string) ([]byte, error) {
	r, err := g.b.NewReader(ctx, key, nil)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (g *GoCloudBucket) SignedDownloadURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := g.b.SignedURL(ctx, key, &blob.SignedURLOptions{
		Method:      "GET",
		Expiry:      ttl,
		ContentType: "",
	})
	if err != nil {
		return "", fmt.Errorf("signed download url failed: %w", err)
	}
	return u, nil
}

func (g *GoCloudBucket) SignedUploadURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := g.b.SignedURL(ctx, key, &blob.SignedURLOptions{
		Method: "PUT",
		Expiry: ttl,
	})
	if err != nil {
		return "", fmt.Errorf("signed upload url failed: %w", err)
	}
	return u, nil
}

func (g *GoCloudBucket) Close() error {
	return g.b.Close()
}
