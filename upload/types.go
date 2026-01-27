package upload

import "github.com/calypr/data-client/common"

type PresignedURLResponse struct {
	GUID string `json:"guid"`
	URL  string `json:"upload_url"`
}

type UploadConfig struct {
	BucketName        string
	NumParallel       int
	ForceMultipart    bool
	IncludeSubDirName bool
	HasMetadata       bool
	ShowProgress      bool
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
