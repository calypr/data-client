package drs

import "github.com/calypr/data-client/hash"

type ChecksumType = hash.ChecksumType
type Checksum = hash.Checksum
type HashInfo = hash.HashInfo

type AccessURL struct {
	URL     string   `json:"url"`
	Headers []string `json:"headers"`
}

type Authorizations struct {
	BearerAuthIssuers []string `json:"bearer_auth_issuers,omitempty"`
}

type AccessMethod struct {
	Type           string          `json:"type"`
	AccessURL      AccessURL       `json:"access_url"`
	AccessID       string          `json:"access_id,omitempty"`
	Cloud          string          `json:"cloud,omitempty"`
	Region         string          `json:"region,omitempty"`
	Available      string          `json:"available,omitempty"`
	Authorizations *Authorizations `json:"authorizations,omitempty"`
}

type Contents struct {
}

type DRSPage struct {
	DRSObjects []DRSObject `json:"drs_objects"`
}

type DRSObjectResult struct {
	Object *DRSObject
	Error  error
}

type DRSObject struct {
	Id            string         `json:"id,omitempty"`
	Name          string         `json:"name"`
	SelfURI       string         `json:"self_uri,omitempty"`
	Size          int64          `json:"size"`
	CreatedTime   string         `json:"created_time,omitempty"`
	UpdatedTime   string         `json:"updated_time,omitempty"`
	Version       string         `json:"version,omitempty"`
	MimeType      string         `json:"mime_type,omitempty"`
	Checksums     hash.HashInfo  `json:"checksums"`
	AccessMethods []AccessMethod `json:"access_methods"`
	Contents      []Contents     `json:"contents,omitempty"`
	Description   string         `json:"description,omitempty"`
	Aliases       []string       `json:"aliases,omitempty"`
}

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
