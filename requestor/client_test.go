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
	if got := formatted.ResourcePaths[0]; got != "/demo/projects/program/data" {
		t.Fatalf("unexpected formatted path: %q", got)
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
