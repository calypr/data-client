package hash

import (
	"encoding/json"
	"testing"
)

func TestChecksumType_IsValid(t *testing.T) {
	tests := []struct {
		name string
		ct   ChecksumType
		want bool
	}{
		{"valid sha256", ChecksumTypeSHA256, true},
		{"valid md5", ChecksumTypeMD5, true},
		{"invalid type", "invalid", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ct.IsValid(); got != tt.want {
				t.Errorf("ChecksumType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHashInfo_UnmarshalJSON_Map(t *testing.T) {
	jsonMap := `{"sha256": "hash-val", "md5": "md5-val"}`
	var h HashInfo
	if err := json.Unmarshal([]byte(jsonMap), &h); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if h.SHA256 != "hash-val" {
		t.Errorf("expected SHA256 hash-val, got %s", h.SHA256)
	}
	if h.MD5 != "md5-val" {
		t.Errorf("expected MD5 md5-val, got %s", h.MD5)
	}
}

func TestHashInfo_UnmarshalJSON_List(t *testing.T) {
	jsonList := `[{"type": "sha256", "checksum": "hash-val"}, {"type": "md5", "checksum": "md5-val"}]`
	var h HashInfo
	if err := json.Unmarshal([]byte(jsonList), &h); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if h.SHA256 != "hash-val" {
		t.Errorf("expected SHA256 hash-val, got %s", h.SHA256)
	}
	if h.MD5 != "md5-val" {
		t.Errorf("expected MD5 md5-val, got %s", h.MD5)
	}
}
