package drs

import (
	"context"

	"github.com/calypr/data-client/hash"
	"github.com/calypr/data-client/request"
)

// Client is the primary interface for interacting with a Calypr DRS server.
// It replaces the legacy IndexdInterface with a modern DRS-first API.
type Client interface {
	request.RequestInterface

	// Metadata retrieval
	GetObject(ctx context.Context, id string) (*DRSObject, error)
	GetObjectByHash(ctx context.Context, checksum *hash.Checksum) ([]DRSObject, error)
	BatchGetObjectsByHash(ctx context.Context, hashes []string) (map[string][]DRSObject, error)

	// Listing
	ListObjects(ctx context.Context) (chan DRSObjectResult, error)
	ListObjectsByProject(ctx context.Context, projectId string) (chan DRSObjectResult, error)
	GetProjectSample(ctx context.Context, projectId string, limit int) ([]DRSObject, error)

	// Mutations
	RegisterRecord(ctx context.Context, record *DRSObject) (*DRSObject, error)
	RegisterRecords(ctx context.Context, records []*DRSObject) ([]*DRSObject, error)
	UpdateRecord(ctx context.Context, updateInfo *DRSObject, did string) (*DRSObject, error)
	DeleteRecord(ctx context.Context, did string) error
	DeleteRecordsByProject(ctx context.Context, projectId string) error

	// Download/URL resolution
	GetDownloadURL(ctx context.Context, id string, accessType string) (*AccessURL, error)

	// Extensions
	// Add an object storage URL to an existing record
	AddURL(ctx context.Context, blobURL, sha256 string, opts ...AddURLOption) (*DRSObject, error)

	// Utility operations
	UpsertRecord(ctx context.Context, url string, sha256 string, fileSize int64, projectId string) (*DRSObject, error)
	BuildDrsObj(fileName string, checksum string, size int64, drsId string) (*DRSObject, error)

	// Runtime context info
	GetProjectId() string
	GetBucketName() string
	GetOrganization() string

	// Fluent configuration
	WithProject(projectId string) Client
	WithOrganization(organization string) Client
	WithBucket(bucketName string) Client
}

type AddURLOption func(map[string]any)
