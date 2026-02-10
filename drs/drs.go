package drs

import (
	"fmt"
	"strings"

	"github.com/calypr/data-client/hash"
	"github.com/google/uuid"
)

// NAMESPACE is the UUID namespace used for generating DRS UUIDs
var NAMESPACE = uuid.NewMD5(uuid.NameSpaceURL, []byte("calypr.org"))

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
	if bucketName == "" {
		return nil, fmt.Errorf("error: bucket name is empty")
	}

	checksum = NormalizeOid(checksum)
	// Standard Gen3-style storage path: s3://bucket/guid/checksum
	fileURL := fmt.Sprintf("s3://%s/%s/%s", bucketName, drsId, checksum)

	authzStr, err := ProjectToResource(org, projectId)
	if err != nil {
		return nil, err
	}
	authorizations := Authorizations{
		BearerAuthIssuers: []string{authzStr},
	}

	drsObj := DRSObject{
		Id:   drsId,
		Name: fileName,
		AccessMethods: []AccessMethod{{
			Type: "s3",
			AccessURL: AccessURL{
				URL: fileURL,
			},
			Authorizations: &authorizations,
		}},
		Checksums: hash.HashInfo{SHA256: checksum},
		Size:      size,
	}

	return &drsObj, nil
}

// ConvertToCandidate converts a DRSObject to a DRSObjectCandidate for registration.
// This is needed because the server expects checksums as an array of Checksum objects,
// while DRSObject uses HashInfo (which marshals to the correct format but has different Go types).
func ConvertToCandidate(obj *DRSObject) DRSObjectCandidate {
	// Convert HashInfo to []Checksum
	var checksums []Checksum
	if obj.Checksums.MD5 != "" {
		checksums = append(checksums, Checksum{Type: hash.ChecksumTypeMD5, Checksum: NormalizeOid(obj.Checksums.MD5)})
	}
	if obj.Checksums.SHA != "" {
		checksums = append(checksums, Checksum{Type: hash.ChecksumTypeSHA1, Checksum: NormalizeOid(obj.Checksums.SHA)})
	}
	if obj.Checksums.SHA256 != "" {
		checksums = append(checksums, Checksum{Type: hash.ChecksumTypeSHA256, Checksum: NormalizeOid(obj.Checksums.SHA256)})
	}
	if obj.Checksums.SHA512 != "" {
		checksums = append(checksums, Checksum{Type: hash.ChecksumTypeSHA512, Checksum: NormalizeOid(obj.Checksums.SHA512)})
	}
	if obj.Checksums.CRC != "" {
		checksums = append(checksums, Checksum{Type: hash.ChecksumTypeCRC32C, Checksum: NormalizeOid(obj.Checksums.CRC)})
	}
	if obj.Checksums.ETag != "" {
		checksums = append(checksums, Checksum{Type: hash.ChecksumTypeETag, Checksum: NormalizeOid(obj.Checksums.ETag)})
	}

	return DRSObjectCandidate{
		Id:            obj.Id,
		Name:          obj.Name,
		Size:          obj.Size,
		Version:       obj.Version,
		MimeType:      obj.MimeType,
		Checksums:     checksums,
		AccessMethods: obj.AccessMethods,
		Contents:      obj.Contents,
		Description:   obj.Description,
		Aliases:       obj.Aliases,
	}
}

func NormalizeOid(oid string) string {
	if strings.HasPrefix(oid, "sha256:") {
		return strings.TrimPrefix(oid, "sha256:")
	}
	return oid
}
