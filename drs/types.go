package drs

import (
	"github.com/calypr/data-client/apigen/drs"
	"github.com/calypr/data-client/hash"
)

type ChecksumType = string
type Checksum = drs.Checksum
type HashInfo = hash.HashInfo

type AccessURL = drs.AccessMethodAccessUrl
type Authorizations = drs.AccessMethodAuthorizations
type AccessMethod = drs.AccessMethod

type Contents = drs.ContentsObject

type DRSPage struct {
	DRSObjects []DRSObject `json:"drs_objects"`
}

type DRSObjectResult struct {
	Object *DRSObject
	Error  error
}

type DRSObject = drs.DrsObject

// DRSObjectCandidate represents a DRS object candidate for registration.
// This matches the server's expected format where checksums is an array of Checksum objects.
// Server-generated fields (id, created_time, updated_time, self_uri) are not included.
type DRSObjectCandidate struct {
	Id            string         `json:"id,omitempty"`
	Name          string         `json:"name,omitempty"`
	Size          int64          `json:"size"`
	Version       string         `json:"version,omitempty"`
	MimeType      string         `json:"mime_type,omitempty"`
	Checksums     []Checksum     `json:"checksums"`
	AccessMethods []AccessMethod `json:"access_methods,omitempty"`
	Contents      []Contents     `json:"contents,omitempty"`
	Description   string         `json:"description,omitempty"`
	Aliases       []string       `json:"aliases,omitempty"`
}

// RegisterObjectsRequest is the request body for registering objects in some DRS implementations.
// This matches the server's API specification.
type RegisterObjectsRequest struct {
	Candidates []DRSObjectCandidate `json:"candidates"`
	Passports  []string             `json:"passports,omitempty"`
}
