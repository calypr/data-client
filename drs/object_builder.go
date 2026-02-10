package drs

import (
	"fmt"
	"strings"

	"github.com/calypr/data-client/hash"
)

type ObjectBuilder struct {
	Bucket       string
	ProjectID    string
	Organization string
	AccessType   string
	PathStyle    string // "CAS" or "" (Gen3 default)
}

func NewObjectBuilder(bucket, projectID string) ObjectBuilder {
	return ObjectBuilder{
		Bucket:     bucket,
		ProjectID:  projectID,
		AccessType: "s3",
		PathStyle:  "Gen3", // Defaults to Gen3 behavior
	}
}

func (b ObjectBuilder) Build(fileName string, checksum string, size int64, drsID string) (*DRSObject, error) {
	if b.Bucket == "" {
		return nil, fmt.Errorf("error: bucket name is empty in config file")
	}
	accessType := b.AccessType
	if accessType == "" {
		accessType = "s3"
	}

	// Remove sha256: prefix if present for clean S3 key
	checksum = strings.TrimPrefix(checksum, "sha256:")

	var fileURL string
	if b.PathStyle == "CAS" {
		// CAS-style (s3://bucket/checksum)
		fileURL = fmt.Sprintf("s3://%s/%s", b.Bucket, checksum)
	} else {
		// Gen3-style (s3://bucket/guid/checksum)
		fileURL = fmt.Sprintf("s3://%s/%s/%s", b.Bucket, drsID, checksum)
	}

	authzStr, err := ProjectToResource(b.Organization, b.ProjectID)
	if err != nil {
		return nil, err
	}
	authorizations := Authorizations{
		BearerAuthIssuers: []string{authzStr},
	}

	drsObj := DRSObject{
		Id:   drsID,
		Name: fileName,
		AccessMethods: []AccessMethod{{
			Type:           accessType,
			AccessURL:      AccessURL{URL: fileURL},
			Authorizations: &authorizations,
		}},
		Checksums: hash.HashInfo{SHA256: checksum},
		Size:      size,
	}

	return &drsObj, nil
}
