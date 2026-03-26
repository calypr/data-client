package common

import (
	"io"
	"net/http"
)

type AccessTokenStruct struct {
	AccessToken string `json:"access_token"`
}

// FileUploadRequestObject defines a object for file upload
type FileUploadRequestObject struct {
	SourcePath   string
	ObjectKey    string
	FileMetadata FileMetadata
	GUID         string
	PresignedURL string
	Bucket       string `json:"bucket,omitempty"`
}

// FileDownloadResponseObject defines a object for file download
type FileDownloadResponseObject struct {
	DownloadPath string
	Filename     string
	GUID         string
	PresignedURL string
	// Range is kept for backward compatibility with resume-download semantics (start offset).
	Range int64
	// RangeStart/RangeEnd provide explicit byte range requests (inclusive).
	RangeStart *int64
	RangeEnd   *int64
	Overwrite  bool
	Skip       bool
	Response   *http.Response
	Writer     io.Writer
}

// FileMetadata defines the metadata accepted by the new object management API, Shepherd
type FileMetadata struct {
	Authz   []string `json:"authz"`
	Aliases []string `json:"aliases"`
	// Metadata is an encoded JSON string of any arbitrary metadata the user wishes to upload.
	Metadata map[string]any `json:"metadata"`
}

// RetryObject defines a object for retry upload
type RetryObject struct {
	SourcePath   string
	ObjectKey    string
	FileMetadata FileMetadata
	GUID         string
	RetryCount   int
	Multipart    bool
	Bucket       string
}

// MultipartUploadInit captures the response needed to upload multipart parts.
type MultipartUploadInit struct {
	GUID     string
	UploadID string
}

// MultipartUploadPart represents an uploaded part that must be completed.
type MultipartUploadPart struct {
	PartNumber int32
	ETag       string
}

type ManifestObject struct {
	GUID      string `json:"object_id"`
	SubjectID string `json:"subject_id"`
	Title     string `json:"title"`
	Size      int64  `json:"size"`
}
 
// ShepherdInitRequestObject represents the payload sent to Shepherd
type ShepherdInitRequestObject struct {
	Filename string         `json:"file_name"`
	Authz    ShepherdAuthz  `json:"authz"`
	Aliases  []string       `json:"aliases"`
	Metadata map[string]any `json:"metadata"`
}
 
type ShepherdAuthz struct {
	Version       string   `json:"version"`
	ResourcePaths []string `json:"resource_paths"`
}
 
type PresignedURLResponse struct {
	GUID string `json:"guid"`
	URL  string `json:"upload_url"`
}
