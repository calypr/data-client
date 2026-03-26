package drs

import (
	"time"

	"github.com/calypr/data-client/apigen/internalapi"
	"github.com/calypr/data-client/hash"
)

// Internal compatibility types for Internal DRS servers.
// These are used internally by DrsClient to communicate with the server's /index and /ga4gh endpoints.

type OutputObject struct {
	Id            string           `json:"id"`
	Name          string           `json:"name"`
	SelfURI       string           `json:"self_uri,omitempty"`
	Size          int64            `json:"size"`
	CreatedTime   string           `json:"created_time,omitempty"`
	UpdatedTime   string           `json:"updated_time,omitempty"`
	Version       string           `json:"version,omitempty"`
	MimeType      string           `json:"mime_type,omitempty"`
	Checksums     []hash.Checksum  `json:"checksums"`
	AccessMethods []AccessMethod   `json:"access_methods"`
	Contents      []Contents       `json:"contents,omitempty"`
	Description   string           `json:"description,omitempty"`
	Aliases       []string         `json:"aliases,omitempty"`
}

func ConvertOutputObjectToDRSObject(in *OutputObject) *DRSObject {
	if in == nil {
		return nil
	}

	drsChecksums := make([]Checksum, len(in.Checksums))
	for i, c := range in.Checksums {
		drsChecksums[i] = Checksum{
			Checksum: c.Checksum,
			Type:     string(c.Type),
		}
	}

	createdTime, _ := time.Parse(time.RFC3339, in.CreatedTime)
	var updatedTimePtr *time.Time
	if ut, err := time.Parse(time.RFC3339, in.UpdatedTime); err == nil {
		updatedTimePtr = &ut
	}

	return &DRSObject{
		Id:            in.Id,
		Name:          internalapi.PtrString(in.Name),
		SelfUri:       in.SelfURI,
		Size:          in.Size,
		CreatedTime:   createdTime,
		UpdatedTime:   updatedTimePtr,
		Version:       internalapi.PtrString(in.Version),
		MimeType:      internalapi.PtrString(in.MimeType),
		Checksums:     drsChecksums,
		AccessMethods: in.AccessMethods,
		Contents:      in.Contents,
		Description:   internalapi.PtrString(in.Description),
		Aliases:       in.Aliases,
	}
}

// InternalRecord embeds InternalRecord for backward compatibility
type InternalRecord struct {
	internalapi.InternalRecord
}

type ListRecords struct {
	Records []OutputInfo `json:"records"`
}

type OutputInfo struct {
	internalapi.InternalRecordResponse
}

// InternalRecordForm is used for legacy /index registration
type InternalRecordForm struct {
	InternalRecord
	Form string `json:"form"`
	Rev  string `json:"rev,omitempty"`
}

func (outputInfo *OutputInfo) ToInternalRecord() *InternalRecord {
	return &InternalRecord{
		InternalRecord: internalapi.InternalRecord{
			Did:      outputInfo.Did,
			Size:     outputInfo.Size,
			FileName: outputInfo.FileName,
			Urls:     outputInfo.Urls,
			Authz:    outputInfo.Authz,
			Hashes:   outputInfo.Hashes,
		},
	}
}

// UpdateInputInfo is the put object for index records
type UpdateInputInfo struct {
	FileName     *string        `json:"file_name,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	URLsMetadata map[string]any `json:"urls_metadata,omitempty"`
	Version      *string        `json:"version,omitempty"`
	URLs         []string       `json:"urls,omitempty"`
	ACL          []string       `json:"acl,omitempty"`
	Authz        []string       `json:"authz,omitempty"`
}
