package drs

import (
	"github.com/calypr/data-client/hash"
	syclient "github.com/calypr/syfon/client"
)

type ChecksumType = string
type Checksum = syclient.Checksum
type HashInfo = hash.HashInfo

type AccessURL = syclient.AccessMethodAccessURL
type Authorizations = syclient.AccessMethodAuthorizations
type AccessMethod = syclient.AccessMethod

type Contents = syclient.ContentsObject

type DRSPage = syclient.DRSPage

type DRSObjectResult struct {
	Object *DRSObject
	Error  error
}

type DRSObject = syclient.DRSObject

type DRSObjectCandidate = syclient.DRSObjectCandidate
type RegisterObjectsRequest = syclient.RegisterObjectsRequest
