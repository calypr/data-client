package upload

import "github.com/calypr/data-client/client/common"

type MultipartPartObject struct {
	PartNumber int    `json:"PartNumber"`
	ETag       string `json:"ETag"`
}

type UploadConfig struct {
	BucketName        string
	NumParallel       int
	ForceMultipart    bool
	IncludeSubDirName bool
	HasMetadata       bool
	ShowProgress      bool
}

// ManifestObject represents an object from manifest that downloaded from windmill / data-portal
type ManifestObject struct {
	ObjectID  string `json:"object_id"`
	SubjectID string `json:"subject_id"`
	Filename  string `json:"file_name"`
	Filesize  int64  `json:"file_size"`
}

// InitRequestObject represents the payload that sends to FENCE for getting a singlepart upload presignedURL or init a multipart upload for new object file
type InitRequestObject struct {
	Filename string `json:"file_name"`
	Bucket   string `json:"bucket,omitempty"`
	GUID     string `json:"guid,omitempty"`
}

// ShepherdInitRequestObject represents the payload that sends to Shepherd for getting a singlepart upload presignedURL or init a multipart upload for new object file
type ShepherdInitRequestObject struct {
	Filename string        `json:"file_name"`
	Authz    ShepherdAuthz `json:"authz"`
	Aliases  []string      `json:"aliases"`
	// Metadata is an encoded JSON string of any arbitrary metadata the user wishes to upload.
	Metadata map[string]any `json:"metadata"`
}
type ShepherdAuthz struct {
	Version       string   `json:"version"`
	ResourcePaths []string `json:"resource_paths"`
}

// MultipartUploadRequestObject represents the payload that sends to FENCE for getting a presignedURL for a part
type MultipartUploadRequestObject struct {
	Key        string `json:"key"`
	UploadID   string `json:"uploadId"`
	PartNumber int    `json:"partNumber"`
	Bucket     string `json:"bucket,omitempty"`
}

// MultipartCompleteRequestObject represents the payload that sends to FENCE for completeing a multipart upload
type MultipartCompleteRequestObject struct {
	Key      string                `json:"key"`
	UploadID string                `json:"uploadId"`
	Parts    []MultipartPartObject `json:"parts"`
	Bucket   string                `json:"bucket,omitempty"`
}

// FileInfo is a helper struct for including subdirname as filename
type FileInfo struct {
	FilePath     string
	Filename     string
	FileMetadata common.FileMetadata
	ObjectId     string
}

// RenamedOrSkippedFileInfo is a helper struct for recording renamed or skipped files
type RenamedOrSkippedFileInfo struct {
	GUID        string
	OldFilename string
	NewFilename string
}
