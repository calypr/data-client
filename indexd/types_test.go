package indexd

import (
	"testing"

	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/hash"
)

func TestConvertOutputObjectToDRSObject(t *testing.T) {
	out := &OutputObject{
		Id:          "did-1",
		Name:        "file.txt",
		SelfURI:     "drs://server/did-1",
		Size:        12345,
		CreatedTime: "2023-01-01T00:00:00Z",
		UpdatedTime: "2023-01-02T00:00:00Z",
		Version:     "v1",
		MimeType:    "text/plain",
		Checksums: []hash.Checksum{
			{Type: hash.ChecksumTypeSHA256, Checksum: "sha256-hash"},
			{Type: hash.ChecksumTypeMD5, Checksum: "md5-hash"},
		},
		AccessMethods: []drs.AccessMethod{
			{
				Type: "s3",
				AccessURL: drs.AccessURL{
					URL: "s3://bucket/key",
				},
			},
		},
		Description: "A test file",
		Aliases:     []string{"alias1"},
	}

	drsObj := ConvertOutputObjectToDRSObject(out)

	if drsObj.Id != out.Id {
		t.Errorf("expected Id %s, got %s", out.Id, drsObj.Id)
	}
	if drsObj.Name != out.Name {
		t.Errorf("expected Name %s, got %s", out.Name, drsObj.Name)
	}
	if drsObj.Size != out.Size {
		t.Errorf("expected Size %d, got %d", out.Size, drsObj.Size)
	}
	// Verify Checksums conversion (slice to HashInfo)
	if drsObj.Checksums.SHA256 != "sha256-hash" {
		t.Errorf("expected SHA256 %s, got %s", "sha256-hash", drsObj.Checksums.SHA256)
	}
	if drsObj.Checksums.MD5 != "md5-hash" {
		t.Errorf("expected MD5 %s, got %s", "md5-hash", drsObj.Checksums.MD5)
	}
	if len(drsObj.AccessMethods) != 1 {
		t.Errorf("expected 1 access method, got %d", len(drsObj.AccessMethods))
	}
	if drsObj.AccessMethods[0].AccessURL.URL != "s3://bucket/key" {
		t.Errorf("expected access URL s3://bucket/key, got %s", drsObj.AccessMethods[0].AccessURL.URL)
	}
}
