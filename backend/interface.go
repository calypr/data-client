package backend

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/calypr/data-client/common"
	drs "github.com/calypr/data-client/drs"
)

// Backend abstract the interaction with underlying data service (Gen3 or standard DRS)
type Backend interface {
	Name() string
	Logger() *slog.Logger

	// --- Read Operations ---

	// GetFileDetails retrieves the DRS object for a given GUID/DID
	GetFileDetails(ctx context.Context, guid string) (*drs.DRSObject, error)

	// GetObjectByHash retrieves objects matching a checksum
	GetObjectByHash(ctx context.Context, checksumType, checksum string) ([]drs.DRSObject, error)

	// BatchGetObjectsByHash retrieves objects matching a list of hashes
	BatchGetObjectsByHash(ctx context.Context, hashes []string) (map[string][]drs.DRSObject, error)

	// GetDownloadURL retrieves a signed URL for downloading the file content
	// accessID is optional (used for DRS objects with multiple access methods)
	GetDownloadURL(ctx context.Context, guid string, accessID string) (string, error)

	// Download performs the HTTP GET for the file content using the backend's preferred request engine.
	Download(ctx context.Context, fdr *common.FileDownloadResponseObject) (*http.Response, error)

	// --- Write Operations ---

	// Register registers a new file metadata record.
	// Returns the registered object (with populated GUID/DID if it was new)
	Register(ctx context.Context, obj *drs.DRSObject) (*drs.DRSObject, error)

	// BatchRegister registers multiple file metadata records.
	BatchRegister(ctx context.Context, objs []*drs.DRSObject) ([]*drs.DRSObject, error)

	// GetUploadURL retrieves a presigned URL for uploading file content.
	// implementation handles provider-specific logic (e.g. Fence vs Shepherd vs DRS-Upload)
	GetUploadURL(ctx context.Context, guid string, filename string, metadata common.FileMetadata, bucket string) (string, error)

	// Upload performs the HTTP PUT for the file content to the presigned URL.
	Upload(ctx context.Context, url string, body io.Reader, size int64) error
}
