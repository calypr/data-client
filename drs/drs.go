package drs

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// NAMESPACE is the UUID namespace used for generating DRS UUIDs
var NAMESPACE = uuid.NewMD5(uuid.NameSpaceURL, []byte("calypr.org"))

func sanitizePathComponent(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "/")
	v = strings.ReplaceAll(v, "\\", "/")
	return strings.ReplaceAll(v, " ", "_")
}

// StoragePrefix returns the bucket key prefix used for object placement.
// Preferred layout is "<organization>/<project>" when organization is provided.
// When organization is empty, it falls back to "<program>/<project>" for hyphenated
// project IDs or "default/<project>" otherwise.
func StoragePrefix(org, project string) string {
	org = sanitizePathComponent(org)
	project = sanitizePathComponent(project)
	if project == "" {
		return ""
	}
	if org != "" {
		return org + "/" + project
	}
	if strings.Contains(project, "-") {
		parts := strings.SplitN(project, "-", 2)
		return sanitizePathComponent(parts[0]) + "/" + sanitizePathComponent(parts[1])
	}
	return "default/" + project
}

func ProjectToResource(org, project string) (string, error) {
	if org != "" {
		return "/programs/" + org + "/projects/" + project, nil
	}
	if project == "" {
		return "", fmt.Errorf("error: project ID is empty")
	}
	if !strings.Contains(project, "-") {
		return "/programs/default/projects/" + project, nil
	}
	projectIdArr := strings.SplitN(project, "-", 2)
	return "/programs/" + projectIdArr[0] + "/projects/" + projectIdArr[1], nil
}

// From git-drs/drsmap/drs_map.go

func DrsUUID(projectId string, hash string) string {
	// create UUID based on project ID and hash
	hashStr := fmt.Sprintf("%s:%s", projectId, hash)
	return uuid.NewSHA1(NAMESPACE, []byte(hashStr)).String()
}

func FindMatchingRecord(records []DRSObject, organization, projectId string) (*DRSObject, error) {
	if len(records) == 0 {
		return nil, nil
	}

	// Convert project ID to resource path format for comparison
	expectedAuthz, err := ProjectToResource(organization, projectId)
	if err != nil {
		return nil, fmt.Errorf("error converting project ID to resource format: %v", err)
	}

	for _, record := range records {
		for _, access := range record.AccessMethods {
			if access.Authorizations == nil {
				continue
			}

			// Check BearerAuthIssuers using a map for O(1) lookup (ref: "lists suck")
			issuersMap := make(map[string]struct{}, len(access.Authorizations.BearerAuthIssuers))
			for _, issuer := range access.Authorizations.BearerAuthIssuers {
				issuersMap[issuer] = struct{}{}
			}

			if _, ok := issuersMap[expectedAuthz]; ok {
				return &record, nil
			}
		}
	}
	return nil, nil
}

// DRS UUID generation using SHA1 (compatible with git-drs)
func GenerateDrsID(projectId, hash string) string {
	return DrsUUID(projectId, hash)
}

func BuildDrsObj(fileName string, checksum string, size int64, drsId string, bucketName string, org string, projectId string) (*DRSObject, error) {
	return BuildDrsObjWithPrefix(fileName, checksum, size, drsId, bucketName, org, projectId, "")
}

func BuildDrsObjWithPrefix(fileName string, checksum string, size int64, drsId string, bucketName string, org string, projectId string, storagePrefix string) (*DRSObject, error) {
	if bucketName == "" {
		return nil, fmt.Errorf("error: bucket name is empty")
	}

	checksum = NormalizeOid(checksum)
	prefix := strings.Trim(strings.TrimSpace(storagePrefix), "/")
	if prefix == "" {
		prefix = StoragePrefix(org, projectId)
	}
	var fileURL string
	// Canonical CAS-style storage path:
	// s3://bucket/{org}/{project}/sha256
	if prefix != "" {
		fileURL = fmt.Sprintf("s3://%s/%s/%s", bucketName, prefix, checksum)
	} else {
		fileURL = fmt.Sprintf("s3://%s/%s", bucketName, checksum)
	}

	authzStr, err := ProjectToResource(org, projectId)
	if err != nil {
		return nil, err
	}
	authorizations := Authorizations{
		BearerAuthIssuers: []string{authzStr},
	}

	drsObj := DRSObject{
		Id:   drsId,
		Name: &fileName,
		AccessMethods: []AccessMethod{{
			Type: "s3",
			AccessUrl: &AccessURL{
				Url: fileURL,
			},
			Authorizations: &authorizations,
		}},
		Checksums: []Checksum{{
			Type: "sha256",
			Checksum: checksum,
		}},
		Size:      size,
	}

	return &drsObj, nil
}

// ConvertToCandidate converts a DRSObject to a DRSObjectCandidate for registration.
// This is needed because the server expects checksums as an array of Checksum objects,
// while DRSObject uses HashInfo (which marshals to the correct format but has different Go types).
func ConvertToCandidate(obj *DRSObject) DRSObjectCandidate {
	var name string
	if obj.Name != nil {
		name = *obj.Name
	}
	var version string
	if obj.Version != nil {
		version = *obj.Version
	}
	var mimeType string
	if obj.MimeType != nil {
		mimeType = *obj.MimeType
	}
	var description string
	if obj.Description != nil {
		description = *obj.Description
	}

	return DRSObjectCandidate{
		Name:          name,
		Size:          obj.Size,
		Version:       version,
		MimeType:      mimeType,
		Checksums:     obj.Checksums,
		AccessMethods: obj.AccessMethods,
		Contents:      nil, // ContentsObject in gen is different
		Description:   description,
		Aliases:       obj.Aliases,
	}
}

func NormalizeOid(oid string) string {
	if strings.HasPrefix(oid, "sha256:") {
		return strings.TrimPrefix(oid, "sha256:")
	}
	return oid
}
