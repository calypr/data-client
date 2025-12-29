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
	FilePath     string
	Filename     string
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
	URL          string
	Range        int64
	Overwrite    bool
	Skip         bool
	Response     *http.Response
	Writer       io.Writer
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
	FilePath     string
	Filename     string
	FileMetadata FileMetadata
	GUID         string
	RetryCount   int
	Multipart    bool
	Bucket       string
}

type ManifestObject struct {
	ObjectID  string `json:"object_id"`
	SubjectID string `json:"subject_id"`
	Title     string `json:"title"`
	Size      int64  `json:"size"`
}
