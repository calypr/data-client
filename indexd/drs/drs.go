package drs

import (
	"fmt"
	"strings"

	"github.com/calypr/data-client/indexd/hash"
	"github.com/google/uuid"
)

// NAMESPACE is the UUID namespace used for generating DRS UUIDs
var NAMESPACE = uuid.NewMD5(uuid.NameSpaceURL, []byte("calypr.org"))

func ProjectToResource(project string) (string, error) {
	if !strings.Contains(project, "-") {
		return "", fmt.Errorf("error: invalid project ID %s, ID should look like <program>-<project>", project)
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

func FindMatchingRecord(records []DRSObject, projectId string) (*DRSObject, error) {
	if len(records) == 0 {
		return nil, nil
	}

	// Convert project ID to resource path format for comparison
	expectedAuthz, err := ProjectToResource(projectId)
	if err != nil {
		return nil, fmt.Errorf("error converting project ID to resource format: %v", err)
	}

	for _, record := range records {
		for _, access := range record.AccessMethods {
			if access.Authorizations != nil && access.Authorizations.Value == expectedAuthz {
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

func BuildDrsObj(fileName string, checksum string, size int64, drsId string, bucketName string, projectId string) (*DRSObject, error) {
	if bucketName == "" {
		return nil, fmt.Errorf("error: bucket name is empty")
	}

	fileURL := fmt.Sprintf("s3://%s/%s/%s", bucketName, drsId, checksum)

	authzStr, err := ProjectToResource(projectId)
	if err != nil {
		return nil, err
	}
	authorizations := Authorizations{
		Value: authzStr,
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
