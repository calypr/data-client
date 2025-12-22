package common

import (
	"io"
	"net/http"

	"github.com/vbauerster/mpb/v8"
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
	Request      *http.Request
	Progress     *mpb.Progress
	Bar          *mpb.Bar
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
	Filename  string `json:"file_name"`
	Filesize  int64  `json:"file_size"`
}
