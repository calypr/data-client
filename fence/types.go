package fence

// MultipartPart represents a part of a multipart upload
type MultipartPart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"ETag"`
}

// FenceResponse represents the standard response from Fence data endpoints
type FenceResponse struct {
	URL          string   `json:"url"`
	UploadURL    string   `json:"upload_url"` // Alias found in some Fence versions
	GUID         string   `json:"guid"`
	UploadID     string   `json:"uploadId"`
	PresignedURL string   `json:"presigned_url"`
	FileName     string   `json:"file_name"`
	URLs         []string `json:"urls"`
	Size         int64    `json:"size"`
}

// InitRequestObject represents the payload for initializing an upload
type InitRequestObject struct {
	Filename string `json:"file_name"`
	Bucket   string `json:"bucket,omitempty"`
	GUID     string `json:"guid,omitempty"`
}

// MultipartUploadRequestObject represents the payload for getting a presigned URL for a part
type MultipartUploadRequestObject struct {
	Key        string `json:"key"`
	UploadID   string `json:"uploadId"`
	PartNumber int    `json:"partNumber"`
	Bucket     string `json:"bucket,omitempty"`
}

// MultipartCompleteRequestObject represents the payload for completing a multipart upload
type MultipartCompleteRequestObject struct {
	Key      string          `json:"key"`
	UploadID string          `json:"uploadId"`
	Parts    []MultipartPart `json:"parts"`
	Bucket   string          `json:"bucket,omitempty"`
}

type S3Bucket struct {
	EndpointURL string `json:"endpoint_url"`
	Region      string `json:"region"`
}

type S3BucketsResponse struct {
	S3Buckets map[string]*S3Bucket `json:"s3_buckets"`
}
