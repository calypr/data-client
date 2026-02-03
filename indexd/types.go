package indexd

import (
	"github.com/calypr/data-client/indexd/drs"
	"github.com/calypr/data-client/indexd/hash"
)

type OutputObject struct {
	Id            string             `json:"id"`
	Name          string             `json:"name"`
	SelfURI       string             `json:"self_uri,omitempty"`
	Size          int64              `json:"size"`
	CreatedTime   string             `json:"created_time,omitempty"`
	UpdatedTime   string             `json:"updated_time,omitempty"`
	Version       string             `json:"version,omitempty"`
	MimeType      string             `json:"mime_type,omitempty"`
	Checksums     []hash.Checksum    `json:"checksums"`
	AccessMethods []drs.AccessMethod `json:"access_methods"`
	Contents      []drs.Contents     `json:"contents,omitempty"`
	Description   string             `json:"description,omitempty"`
	Aliases       []string           `json:"aliases,omitempty"`
}

func ConvertOutputObjectToDRSObject(in *OutputObject) *drs.DRSObject {
	if in == nil {
		return nil
	}

	hashInfo := hash.ConvertChecksumsToHashInfo(in.Checksums)

	return &drs.DRSObject{
		Id:            in.Id,
		Name:          in.Name,
		SelfURI:       in.SelfURI,
		Size:          in.Size,
		CreatedTime:   in.CreatedTime,
		UpdatedTime:   in.UpdatedTime,
		Version:       in.Version,
		MimeType:      in.MimeType,
		Checksums:     hashInfo,
		AccessMethods: in.AccessMethods,
		Contents:      in.Contents,
		Description:   in.Description,
		Aliases:       in.Aliases,
	}
}

// UpdateInputInfo is the put object for index records
type UpdateInputInfo struct {
	// Human-readable file name
	FileName string `json:"file_name,omitempty"`

	// Additional metadata as key-value pairs
	Metadata map[string]any `json:"metadata,omitempty"`

	// URL-specific metadata as key-value pairs
	URLsMetadata map[string]any `json:"urls_metadata,omitempty"`

	// Version of the record
	Version string `json:"version,omitempty"`

	// List of URLs where the file can be accessed
	URLs []string `json:"urls,omitempty"`

	// List of access control lists (ACLs)
	ACL []string `json:"acl,omitempty"`

	// List of authorization policies
	Authz []string `json:"authz,omitempty"`
}

type S3Meta struct {
	Size         int64
	LastModified string
}
