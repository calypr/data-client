package arborist

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/calypr/data-client/request"
)

type fakeRequest struct {
	lastBody   map[string]any
	lastMethod string
	lastPath   string
	lastQuery  string
	statusCode int
}

func (f *fakeRequest) New(method, rawURL string) *request.RequestBuilder {
	return (&request.Request{}).New(method, rawURL)
}

func (f *fakeRequest) Do(ctx context.Context, rb *request.RequestBuilder) (*http.Response, error) {
	f.lastMethod = rb.Method
	parsed, _ := url.Parse(rb.Url)
	f.lastPath = parsed.Path
	f.lastQuery = parsed.RawQuery
	if rb.Body != nil {
		body, _ := io.ReadAll(rb.Body)
		_ = json.Unmarshal(body, &f.lastBody)
	}
	status := f.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
	}, nil
}

func TestGrantOrgMembership(t *testing.T) {
	req := &fakeRequest{}
	client := &Client{RequestInterface: req, Endpoint: "https://example.org"}

	if err := client.GrantOrgMembership(context.Background(), "Ellrott_Lab", "USER@OHSU.EDU", ""); err != nil {
		t.Fatalf("grant membership: %v", err)
	}
	if req.lastMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", req.lastMethod)
	}
	if req.lastPath != "/authz/access/user" {
		t.Fatalf("expected access user path, got %s", req.lastPath)
	}
	if req.lastBody["resource_path"] != "/programs/Ellrott_Lab/projects" {
		t.Fatalf("expected org projects resource path, got %#v", req.lastBody["resource_path"])
	}
	if req.lastBody["username"] != "user@ohsu.edu" {
		t.Fatalf("expected normalized username, got %#v", req.lastBody["username"])
	}
	if req.lastBody["role_id"] != "org-member" {
		t.Fatalf("expected default org-member role, got %#v", req.lastBody["role_id"])
	}
}

func TestRevokeOrgMembership(t *testing.T) {
	req := &fakeRequest{}
	client := &Client{RequestInterface: req, Endpoint: "https://example.org"}

	if err := client.RevokeOrgMembership(context.Background(), "cbds", "user@ohsu.edu", "writer"); err != nil {
		t.Fatalf("revoke membership: %v", err)
	}
	if req.lastMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", req.lastMethod)
	}
	if req.lastPath != "/authz/access/user" {
		t.Fatalf("expected access user path, got %s", req.lastPath)
	}
	if req.lastBody["resource_path"] != "/programs/cbds/projects" {
		t.Fatalf("expected org projects resource path, got %#v", req.lastBody["resource_path"])
	}
	if req.lastBody["role_id"] != "writer" {
		t.Fatalf("expected explicit role, got %#v", req.lastBody["role_id"])
	}
}

func TestOrgMembershipRejectsResourcePath(t *testing.T) {
	if _, err := NewOrgMembershipRequest("/programs/cbds", "user@ohsu.edu", "owner"); err == nil {
		t.Fatal("expected resource path organization to fail")
	}
}

func TestRouteFamilies(t *testing.T) {
	tests := []struct {
		name       string
		call       func(*Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
	}{
		{
			name:       "auth mapping",
			call:       func(c *Client) error { return c.AuthMapping(context.Background(), nil) },
			wantMethod: http.MethodGet,
			wantPath:   "/authz/mapping",
		},
		{
			name: "create owned descendant",
			call: func(c *Client) error {
				return c.CreateOwnedDescendant(context.Background(), CreateOwnedDescendantRequest{Name: "proj", ParentPath: "/programs/cbds/projects"}, nil)
			},
			wantMethod: http.MethodPost,
			wantPath:   "/authz/ownership/descendant",
		},
		{
			name: "get ownership resource",
			call: func(c *Client) error {
				return c.GetOwnershipResource(context.Background(), "/programs/cbds/projects/demo", true, true, nil)
			},
			wantMethod: http.MethodGet,
			wantPath:   "/authz/ownership/resource",
			wantQuery:  "include_admins=true&include_children=true&resource_path=%2Fprograms%2Fcbds%2Fprojects%2Fdemo",
		},
		{
			name: "grant access user",
			call: func(c *Client) error {
				return c.GrantAccessUser(context.Background(), AccessUserRequest{
					ResourcePath: "/programs/cbds/projects/demo",
					Username:     "USER@OHSU.EDU",
					RoleID:       "writer",
				}, nil)
			},
			wantMethod: http.MethodPost,
			wantPath:   "/authz/access/user",
		},
		{
			name: "revoke access user",
			call: func(c *Client) error {
				return c.RevokeAccessUser(context.Background(), AccessUserRequest{
					ResourcePath: "/programs/cbds/projects/demo",
					Username:     "USER@OHSU.EDU",
					RoleID:       "writer",
				})
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/authz/access/user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &fakeRequest{}
			client := &Client{RequestInterface: req, Endpoint: "https://example.org"}
			if err := tt.call(client); err != nil {
				t.Fatalf("call failed: %v", err)
			}
			if req.lastMethod != tt.wantMethod {
				t.Fatalf("expected method %s, got %s", tt.wantMethod, req.lastMethod)
			}
			if req.lastPath != tt.wantPath {
				t.Fatalf("expected path %s, got %s", tt.wantPath, req.lastPath)
			}
			if req.lastQuery != tt.wantQuery {
				t.Fatalf("expected query %s, got %s", tt.wantQuery, req.lastQuery)
			}
		})
	}
}

func TestAccessUserNormalizesRequest(t *testing.T) {
	req := &fakeRequest{}
	client := &Client{RequestInterface: req, Endpoint: "https://example.org"}

	if err := client.GrantAccessUser(context.Background(), AccessUserRequest{
		ResourcePath: " /programs/cbds/projects/demo ",
		Username:     " USER@OHSU.EDU ",
		RoleID:       " writer ",
	}, nil); err != nil {
		t.Fatalf("grant access user: %v", err)
	}

	if req.lastBody["resource_path"] != "/programs/cbds/projects/demo" {
		t.Fatalf("expected normalized resource path, got %#v", req.lastBody["resource_path"])
	}
	if req.lastBody["username"] != "user@ohsu.edu" {
		t.Fatalf("expected normalized username, got %#v", req.lastBody["username"])
	}
	if req.lastBody["role_id"] != "writer" {
		t.Fatalf("expected normalized role id, got %#v", req.lastBody["role_id"])
	}
}
