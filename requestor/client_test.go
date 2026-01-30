package requestor

import (
	"testing"
)

func TestGetPolicyKey(t *testing.T) {
	c := &RequestorClient{}

	p1 := CreateRequestRequest{
		PolicyID:      "p1",
		RoleIDs:       []string{"reader"},
		ResourcePaths: []string{"/path1"},
	}
	p2 := CreateRequestRequest{
		PolicyID:      "p1",
		RoleIDs:       []string{"reader"},
		ResourcePaths: []string{"/path1"},
	}
	p3 := CreateRequestRequest{
		PolicyID:      "p2",
		RoleIDs:       []string{"reader"},
		ResourcePaths: []string{"/path1"},
	}

	if c.getPolicyKey(p1) != c.getPolicyKey(p2) {
		t.Errorf("Expected p1 and p2 to have same key")
	}
	if c.getPolicyKey(p1) == c.getPolicyKey(p3) {
		t.Errorf("Expected p1 and p3 to have different keys (PolicyID differs)")
	}

	p4 := CreateRequestRequest{
		RoleIDs:       []string{"a", "b"},
		ResourcePaths: []string{"/p1", "/p2"},
	}
	p5 := CreateRequestRequest{
		RoleIDs:       []string{"b", "a"},
		ResourcePaths: []string{"/p2", "/p1"},
	}
	if c.getPolicyKey(p4) != c.getPolicyKey(p5) {
		t.Errorf("Expected p4 and p5 to have same key (sorting check)")
	}

	// Empty PolicyID check
	p6 := CreateRequestRequest{
		RoleIDs:       []string{"reader"},
		ResourcePaths: []string{"/path1"},
	}
	p7 := CreateRequestRequest{
		RoleIDs:       []string{"reader"},
		ResourcePaths: []string{"/path1"},
	}
	if c.getPolicyKey(p6) != c.getPolicyKey(p7) {
		t.Errorf("Expected p6 and p7 (empty PolicyID) to have same key")
	}
}
