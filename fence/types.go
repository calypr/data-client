package fence

// MultipartPart represents a part of a multipart upload
type MultipartPart struct {
	PartNumber int    `json:"PartNumber"`
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
	PartNumber int    `json:"PartNumber"`
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
	EndpointURL string   `json:"endpoint_url"`
	Programs    []string `json:"programs,omitempty"`
	Region      string   `json:"region"`
}

type S3BucketsResponse struct {
	GSBuckets map[string]any       `json:"GS_BUCKETS,omitempty"`
	S3Buckets map[string]*S3Bucket `json:"S3_BUCKETS,omitempty"`
	// Some versions of fence use lowercase
	S3BucketsLower map[string]*S3Bucket `json:"s3_buckets,omitempty"`
}

type UserPermission struct {
	Method  string `json:"method"`
	Service string `json:"service"`
}

type FenceUserResp struct {
	Active                      bool                        `json:"active"`
	Authz                       map[string][]UserPermission `json:"authz"`
	Azp                         *string                     `json:"azp"`
	CertificatesUploaded        []any                       `json:"certificates_uploaded"`
	DisplayName                 string                      `json:"display_name"`
	Email                       string                      `json:"email"`
	Ga4GhPassportV1             []any                       `json:"ga4gh_passport_v1"`
	Groups                      []any                       `json:"groups"`
	Idp                         string                      `json:"idp"`
	IsAdmin                     bool                        `json:"is_admin"`
	Message                     string                      `json:"message"`
	Name                        string                      `json:"name"`
	PhoneNumber                 string                      `json:"phone_number"`
	PreferredUsername           string                      `json:"preferred_username"`
	PrimaryGoogleServiceAccount *string                     `json:"primary_google_service_account"`
	ProjectAccess               map[string]any              `json:"project_access"`
	Resources                   []string                    `json:"resources"`
	ResourcesGranted            []any                       `json:"resources_granted"`
	Role                        string                      `json:"role"`
	Sub                         string                      `json:"sub"`
	UserID                      int                         `json:"user_id"`
	Username                    string                      `json:"username"`
}

type PingResp struct {
	Profile        string            `yaml:"profile" json:"profile"`
	Username       string            `yaml:"username" json:"username"`
	Endpoint       string            `yaml:"endpoint" json:"endpoint"`
	BucketPrograms map[string]string `yaml:"bucket_programs" json:"bucket_programs"`
	YourAccess     map[string]string `yaml:"your_access" json:"your_access"`
}
