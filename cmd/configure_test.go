package cmd

import (
	"testing"

	"github.com/calypr/data-client/conf"
)

func TestMergeImportedCredentialPreservesExplicitAPIEndpoint(t *testing.T) {
	target := &conf.Credential{
		Profile:     "dev",
		APIEndpoint: "https://explicit.example.org",
		AccessToken: "stale-token",
	}
	imported := &conf.Credential{
		KeyID:       "key-id",
		APIKey:      "api-key",
		APIEndpoint: "https://embedded.example.org",
	}

	mergeImportedCredential(target, imported)

	if target.KeyID != "key-id" {
		t.Fatalf("KeyID = %q, want %q", target.KeyID, "key-id")
	}
	if target.APIKey != "api-key" {
		t.Fatalf("APIKey = %q, want %q", target.APIKey, "api-key")
	}
	if target.APIEndpoint != "https://explicit.example.org" {
		t.Fatalf("APIEndpoint = %q, want explicit endpoint", target.APIEndpoint)
	}
	if target.AccessToken != "" {
		t.Fatalf("AccessToken = %q, want empty", target.AccessToken)
	}
}

func TestMergeImportedCredentialUsesImportedAPIEndpointWhenMissing(t *testing.T) {
	target := &conf.Credential{Profile: "dev"}
	imported := &conf.Credential{APIEndpoint: "https://embedded.example.org"}

	mergeImportedCredential(target, imported)

	if target.APIEndpoint != "https://embedded.example.org" {
		t.Fatalf("APIEndpoint = %q, want %q", target.APIEndpoint, "https://embedded.example.org")
	}
}
