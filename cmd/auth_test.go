package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestAuthPermissionSignatureGroupsArboristPermissions(t *testing.T) {
	signature := authPermissionSignature([]any{
		map[string]any{"method": "read", "service": "*"},
		map[string]any{"method": "create", "service": "*"},
		map[string]any{"method": "*", "service": "indexd"},
		map[string]any{"method": "read", "service": "requestor"},
	})

	want := "*: create, read; indexd: *; requestor: read"
	if signature != want {
		t.Fatalf("signature = %q, want %q", signature, want)
	}
}

func TestAuthPermissionSignatureGroupsFenceProjectAccess(t *testing.T) {
	signature := authPermissionSignature([]any{"write", "read", "read"})

	want := "read, read, write"
	if signature != want {
		t.Fatalf("signature = %q, want %q", signature, want)
	}
}

func TestWriteAuthSummaryCondensesRepeatedPermissionSets(t *testing.T) {
	resourceAccess := map[string]any{
		"/programs/a/projects/one": []any{
			map[string]any{"method": "read", "service": "*"},
			map[string]any{"method": "create", "service": "*"},
		},
		"/programs/a/projects/two": []any{
			map[string]any{"method": "create", "service": "*"},
			map[string]any{"method": "read", "service": "*"},
		},
		"/data_file": []any{
			map[string]any{"method": "*", "service": "indexd"},
		},
	}

	var out bytes.Buffer
	writeAuthSummary(&out, "https://example.org", resourceAccess, false)
	got := out.String()

	for _, want := range []string{
		"Access for https://example.org",
		"3 resources in 2 permission groups",
		"2 resources: *: create, read",
		"  /programs/a/projects/one",
		"  /programs/a/projects/two",
		"1 resource: indexd: *",
		"  /data_file",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q:\n%s", want, got)
		}
	}
}

func TestWriteAuthSummaryCapsLongGroups(t *testing.T) {
	resourceAccess := make(map[string]any)
	for i := 0; i < authSummaryResourceLimit+2; i++ {
		resourceAccess[string(rune('a'+i))] = []any{
			map[string]any{"method": "read", "service": "*"},
		}
	}

	var out bytes.Buffer
	writeAuthSummary(&out, "https://example.org", resourceAccess, false)
	got := out.String()

	if !strings.Contains(got, "... 2 more (use --all to show every resource)") {
		t.Fatalf("summary did not cap long group:\n%s", got)
	}
}
