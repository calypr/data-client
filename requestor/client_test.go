package requestor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/calypr/data-client/request"
)

type fakeRequest struct {
	doFn func(rb *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeRequest) New(method, u string) *request.RequestBuilder {
	return &request.RequestBuilder{Method: method, Url: u, Headers: map[string]string{}}
}

func (f *fakeRequest) Do(ctx context.Context, rb *request.RequestBuilder) (*http.Response, error) {
	return f.doFn(rb)
}

func jsonResponse(status int, v any) *http.Response {
	var body io.ReadCloser = http.NoBody
	if v != nil {
		buf, _ := json.Marshal(v)
		body = io.NopCloser(bytes.NewReader(buf))
	}
	return &http.Response{StatusCode: status, Body: body}
}

func TestRequestorClientStatusError(t *testing.T) {
	err := request.StatusError(jsonResponse(http.StatusBadRequest, map[string]string{"error": "bad request"}), "failed")
	if err == nil || !strings.Contains(err.Error(), "failed: status 400") {
		t.Fatalf("unexpected error: %v", err)
	}
}

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

func TestLoadPoliciesAndFormatPolicy(t *testing.T) {
	policies, err := loadPolicies("add-user-read.yaml")
	if err != nil {
		t.Fatalf("loadPolicies returned error: %v", err)
	}
	if len(policies) == 0 {
		t.Fatal("expected add-user-read.yaml to contain at least one policy")
	}

	if _, err := loadPolicies("missing.yaml"); err == nil {
		t.Fatal("expected missing policy file to return an error")
	}

	formatted := formatPolicy(
		CreateRequestRequest{
			PolicyID:            "policy-1",
			ResourcePaths:       []string{"/PROGRAM/projects/PROJECT/data"},
			ResourceDisplayName: "original",
		},
		"demo-program-demo-project",
		"alice",
	)
	if formatted.Username != "alice" {
		t.Fatalf("expected username to be replaced, got %q", formatted.Username)
	}
	if formatted.ResourceDisplayName != "demo-program-demo-project" {
		t.Fatalf("expected display name to be replaced, got %q", formatted.ResourceDisplayName)
	}
	if got := formatted.ResourcePaths[0]; got != "/demo/projects/program-demo-project/data" {
		t.Fatalf("unexpected formatted path: %q", got)
	}
}

func TestParseProjectResources(t *testing.T) {
	resources, err := ParseProjectResources(`denied resource paths: /programs/HTAN_INT/projects/BForePC, /programs/cbds/projects/git_drs_test, aced/evotypes`)
	if err != nil {
		t.Fatalf("ParseProjectResources returned error: %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources, got %+v", resources)
	}
	want := []string{
		"/programs/HTAN_INT/projects/BForePC",
		"/programs/cbds/projects/git_drs_test",
		"/programs/aced/projects/evotypes",
	}
	for i, expected := range want {
		if resources[i].ResourcePath != expected {
			t.Fatalf("resource %d = %q, want %q", i, resources[i].ResourcePath, expected)
		}
	}
}

func TestParseProjectResourcesFromScopeMessage(t *testing.T) {
	resources, err := ParseProjectResources(`denied organization/project scopes: HTAN_INT/BForePC, cbds/git_drs_test`)
	if err != nil {
		t.Fatalf("ParseProjectResources returned error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %+v", resources)
	}
	if got := resources[0].ResourcePath; got != "/programs/HTAN_INT/projects/BForePC" {
		t.Fatalf("unexpected first resource: %q", got)
	}
}

func TestParseProjectResourcesFromPreflightCopyPasteBlock(t *testing.T) {
	raw := `migration import preflight failed: missing create access for 1320/82628 records across 323 scopes
first denied record: 04ae835f-9a18-5867-8a63-fa527dedf425

Copy/paste scope list:
Ellrott_Lab/embedding_rotation, HTAN_INT/TestUpload1, cbds/017549802cd04d108524ae3f196ffacd

2026/05/01 12:01:42 ERROR command execution failed err="target authorization preflight failed: missing create access"`

	resources, err := ParseProjectResources(raw)
	if err != nil {
		t.Fatalf("ParseProjectResources returned error: %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources, got %+v", resources)
	}
	want := []string{
		"/programs/Ellrott_Lab/projects/embedding_rotation",
		"/programs/HTAN_INT/projects/TestUpload1",
		"/programs/cbds/projects/017549802cd04d108524ae3f196ffacd",
	}
	for i, expected := range want {
		if resources[i].ResourcePath != expected {
			t.Fatalf("resource %d = %q, want %q", i, resources[i].ResourcePath, expected)
		}
	}
}

func TestRequestorClientListCreateAndUpdate(t *testing.T) {
	client := &RequestorClient{
		RequestInterface: &fakeRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				u, err := url.Parse(rb.Url)
				if err != nil {
					return nil, err
				}

				switch {
				case rb.Method == http.MethodGet && u.Path == "/requestor/request":
					if !u.Query().Has("active") {
						t.Fatalf("expected active query parameter")
					}
					if got := u.Query().Get("username"); got != "bob" {
						t.Fatalf("expected username query bob, got %q", got)
					}
					return jsonResponse(http.StatusOK, []Request{{RequestID: "req-list", Status: "active"}}), nil
				case rb.Method == http.MethodPost && u.Path == "/requestor/request":
					body, _ := io.ReadAll(rb.Body)
					var payload CreateRequestRequest
					if err := json.Unmarshal(body, &payload); err != nil {
						t.Fatalf("decode create payload: %v", err)
					}
					if payload.PolicyID == "" {
						t.Fatal("expected create payload policy id")
					}
					return jsonResponse(http.StatusOK, Request{RequestID: "req-create", Status: "open"}), nil
				case rb.Method == http.MethodPut && strings.HasPrefix(u.Path, "/requestor/request/"):
					body, _ := io.ReadAll(rb.Body)
					var payload UpdateRequestRequest
					if err := json.Unmarshal(body, &payload); err != nil {
						t.Fatalf("decode update payload: %v", err)
					}
					if payload.Status != "approved" {
						t.Fatalf("expected approved status, got %q", payload.Status)
					}
					return jsonResponse(http.StatusOK, Request{RequestID: "req-update", Status: payload.Status}), nil
				default:
					return jsonResponse(http.StatusNotFound, nil), nil
				}
			},
		},
		Endpoint: "https://example.org",
	}

	list, err := client.ListRequests(context.Background(), false, true, "bob")
	if err != nil {
		t.Fatalf("ListRequests failed: %v", err)
	}
	if len(list) != 1 || list[0].RequestID != "req-list" {
		t.Fatalf("unexpected list response: %+v", list)
	}

	created, err := client.CreateRequest(context.Background(), CreateRequestRequest{
		PolicyID:      "policy-1",
		ResourcePaths: []string{"/path"},
		RoleIDs:       []string{"reader"},
	}, false)
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}
	if created.RequestID != "req-create" {
		t.Fatalf("unexpected create response: %+v", created)
	}

	updated, err := client.UpdateRequest(context.Background(), "req-1", "approved")
	if err != nil {
		t.Fatalf("UpdateRequest failed: %v", err)
	}
	if updated.Status != "approved" {
		t.Fatalf("unexpected update response: %+v", updated)
	}
}

func TestRequestorClientAddAndRemoveUser(t *testing.T) {
	var revokeCount int

	client := &RequestorClient{
		RequestInterface: &fakeRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				u, err := url.Parse(rb.Url)
				if err != nil {
					return nil, err
				}
				if rb.Method != http.MethodPost || u.Path != "/requestor/request" {
					return jsonResponse(http.StatusNotFound, nil), nil
				}
				if u.Query().Has("revoke") {
					revokeCount++
				}
				return jsonResponse(http.StatusOK, Request{RequestID: "req-ok", Status: "open"}), nil
			},
		},
		Endpoint: "https://example.org",
	}

	added, err := client.AddUser(context.Background(), "demo-program-demo-project", "bob", true, true)
	if err != nil {
		t.Fatalf("AddUser failed: %v", err)
	}
	if len(added) == 0 {
		t.Fatal("expected AddUser to create at least one request")
	}

	revoked, err := client.RemoveUser(context.Background(), "demo-program-demo-project", "bob")
	if err != nil {
		t.Fatalf("RemoveUser failed: %v", err)
	}
	if len(revoked) == 0 {
		t.Fatal("expected RemoveUser to create at least one revocation request")
	}
	if revokeCount == 0 {
		t.Fatal("expected revoke query parameter to be seen for revocation requests")
	}
}

func TestRequestorClientAddUserPreservesEmbeddedDashInProject(t *testing.T) {
	var payloads []CreateRequestRequest

	client := &RequestorClient{
		RequestInterface: &fakeRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				u, err := url.Parse(rb.Url)
				if err != nil {
					return nil, err
				}
				if rb.Method != http.MethodPost || u.Path != "/requestor/request" {
					return jsonResponse(http.StatusNotFound, nil), nil
				}

				var payload CreateRequestRequest
				if err := json.NewDecoder(rb.Body).Decode(&payload); err != nil {
					return nil, err
				}
				payloads = append(payloads, payload)
				return jsonResponse(http.StatusOK, Request{RequestID: "req-ok", Status: "open"}), nil
			},
		},
		Endpoint: "https://example.org",
	}

	created, err := client.AddUser(context.Background(), "cbds-aaa-bbb", "bob@example.org", true, true)
	if err != nil {
		t.Fatalf("AddUser failed: %v", err)
	}
	if len(created) == 0 {
		t.Fatal("expected AddUser to create requests")
	}
	if len(payloads) == 0 {
		t.Fatal("expected request payloads to be captured")
	}

	wantPath := "/programs/cbds/projects/aaa-bbb"
	foundProjectScopedPayload := false
	for _, payload := range payloads {
		if payload.ResourceDisplayName != "cbds-aaa-bbb" {
			t.Fatalf("unexpected display name: %+v", payload)
		}

		hasProjectPlaceholderPath := false
		found := false
		for _, path := range payload.ResourcePaths {
			if strings.Contains(path, "/programs/") {
				hasProjectPlaceholderPath = true
			}
			if path == wantPath || strings.Contains(path, wantPath+"/") {
				found = true
				break
			}
		}
		if hasProjectPlaceholderPath && !found {
			t.Fatalf("expected payload to target %q, got %+v", wantPath, payload.ResourcePaths)
		}
		if found {
			foundProjectScopedPayload = true
		}
	}
	if !foundProjectScopedPayload {
		t.Fatalf("expected at least one project-scoped payload to target %q", wantPath)
	}
}

func TestAddUserToResourcesCreatesRequestsPerResource(t *testing.T) {
	var payloads []CreateRequestRequest
	client := &RequestorClient{
		RequestInterface: &fakeRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				if rb.Method != http.MethodPost {
					return jsonResponse(http.StatusMethodNotAllowed, nil), nil
				}
				var payload CreateRequestRequest
				if err := json.NewDecoder(rb.Body).Decode(&payload); err != nil {
					return nil, err
				}
				payloads = append(payloads, payload)
				return jsonResponse(http.StatusOK, Request{RequestID: "req-ok", Status: "open"}), nil
			},
		},
		Endpoint: "https://example.org",
	}

	resources, err := ParseProjectResources("/programs/cbds/projects/git_drs_test, /programs/aced/projects/evotypes")
	if err != nil {
		t.Fatalf("ParseProjectResources returned error: %v", err)
	}
	created, err := client.AddUserToResources(context.Background(), resources, "bob@example.org", true, true)
	if err != nil {
		t.Fatalf("AddUserToResources failed: %v", err)
	}
	if len(created) != 5 {
		t.Fatalf("expected 5 deduplicated requests, got %d", len(created))
	}

	seen := map[string]bool{}
	for _, payload := range payloads {
		if payload.Username != "bob@example.org" {
			t.Fatalf("unexpected username in payload: %+v", payload)
		}
		seen[strings.Join(payload.RoleIDs, ",")+"|"+strings.Join(payload.ResourcePaths, ",")] = true
	}
	if !seen["reader|/programs/cbds/projects/git_drs_test"] {
		t.Fatalf("missing reader request for cbds/git_drs_test: %+v", payloads)
	}
	if !seen["writer|/programs/aced/projects/evotypes"] {
		t.Fatalf("missing writer request for aced/evotypes: %+v", payloads)
	}
	if !seen["guppy_admin_user|/guppy_admin"] {
		t.Fatalf("missing deduplicated guppy admin request: %+v", payloads)
	}
}
