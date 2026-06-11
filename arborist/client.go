package arborist

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/calypr/calypr-cli/conf"
	"github.com/calypr/calypr-cli/request"
)

const DefaultOrgMemberRole = "org-member"

type Client struct {
	request.RequestInterface
	Endpoint string
}

type ClientInterface interface {
	AuthMapping(ctx context.Context, out any) error
	CreateOwnedDescendant(ctx context.Context, req CreateOwnedDescendantRequest, out any) error
	AddOwner(ctx context.Context, req OwnerMutationRequest, out any) error
	RemoveOwner(ctx context.Context, req OwnerMutationRequest) error
	GetOwnershipResource(ctx context.Context, resourcePath string, includeChildren bool, includeAdmins bool, out any) error
	GrantAccessUser(ctx context.Context, req AccessUserRequest, out any) error
	RevokeAccessUser(ctx context.Context, req AccessUserRequest) error
	GrantOrgMembership(ctx context.Context, organization string, username string, roleID string) error
	RevokeOrgMembership(ctx context.Context, organization string, username string, roleID string) error
}

type CreateOwnedDescendantRequest struct {
	ParentPath  string `json:"parent_path"`
	Name        string `json:"name"`
	Template    string `json:"template,omitempty"`
	Description string `json:"description,omitempty"`
}

type OwnerMutationRequest struct {
	ResourcePath string `json:"resource_path"`
	Username     string `json:"username"`
}

type AccessUserRequest struct {
	ResourcePath string `json:"resource_path"`
	Username     string `json:"username"`
	RoleID       string `json:"role_id"`
}

type errorResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

func (e *errorResponse) ErrorMessage() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return e.Error
}

func NewClient(req request.RequestInterface, creds *conf.Credential) ClientInterface {
	return &Client{
		RequestInterface: req,
		Endpoint:         creds.APIEndpoint,
	}
}

func (c *Client) AuthMapping(ctx context.Context, out any) error {
	return c.do(ctx, http.MethodGet, "authz/mapping", nil, out, "get auth mapping")
}

func (c *Client) CreateOwnedDescendant(ctx context.Context, req CreateOwnedDescendantRequest, out any) error {
	return c.do(ctx, http.MethodPost, "authz/ownership/descendant", req, out, "create owned descendant")
}

func (c *Client) AddOwner(ctx context.Context, req OwnerMutationRequest, out any) error {
	return c.do(ctx, http.MethodPost, "authz/ownership/owner", req, out, "add owner")
}

func (c *Client) RemoveOwner(ctx context.Context, req OwnerMutationRequest) error {
	return c.do(ctx, http.MethodDelete, "authz/ownership/owner", req, nil, "remove owner")
}

func (c *Client) GetOwnershipResource(ctx context.Context, resourcePath string, includeChildren bool, includeAdmins bool, out any) error {
	query := url.Values{}
	query.Set("resource_path", strings.TrimSpace(resourcePath))
	if includeChildren {
		query.Set("include_children", "true")
	}
	if includeAdmins {
		query.Set("include_admins", "true")
	}
	return c.do(ctx, http.MethodGet, "authz/ownership/resource?"+query.Encode(), nil, out, "get ownership resource")
}

func (c *Client) GrantAccessUser(ctx context.Context, req AccessUserRequest, out any) error {
	return c.do(ctx, http.MethodPost, "authz/access/user", normalizeAccessUserRequest(req), out, "grant direct user access")
}

func (c *Client) RevokeAccessUser(ctx context.Context, req AccessUserRequest) error {
	return c.do(ctx, http.MethodDelete, "authz/access/user", normalizeAccessUserRequest(req), nil, "revoke direct user access")
}

func (c *Client) GrantOrgMembership(ctx context.Context, organization string, username string, roleID string) error {
	payload, err := NewOrgMembershipRequest(organization, username, roleID)
	if err != nil {
		return err
	}
	return c.GrantAccessUser(ctx, payload, nil)
}

func (c *Client) RevokeOrgMembership(ctx context.Context, organization string, username string, roleID string) error {
	payload, err := NewOrgMembershipRequest(organization, username, roleID)
	if err != nil {
		return err
	}
	return c.RevokeAccessUser(ctx, payload)
}

func NewOrgMembershipRequest(organization string, username string, roleID string) (AccessUserRequest, error) {
	organization = strings.Trim(strings.TrimSpace(organization), "/")
	username = strings.ToLower(strings.TrimSpace(username))
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		roleID = DefaultOrgMemberRole
	}
	if organization == "" {
		return AccessUserRequest{}, fmt.Errorf("organization is required")
	}
	if strings.Contains(organization, "/") {
		return AccessUserRequest{}, fmt.Errorf("organization must be a single Calypr organization name, not a resource path")
	}
	if username == "" {
		return AccessUserRequest{}, fmt.Errorf("username is required")
	}
	return AccessUserRequest{
		ResourcePath: "/programs/" + organization + "/projects",
		Username:     username,
		RoleID:       roleID,
	}, nil
}

func normalizeAccessUserRequest(req AccessUserRequest) AccessUserRequest {
	req.ResourcePath = strings.TrimSpace(req.ResourcePath)
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.RoleID = strings.TrimSpace(req.RoleID)
	return req
}

func (c *Client) do(ctx context.Context, method string, apiPath string, body any, out any, action string) error {
	url := request.JoinURL(c.Endpoint, apiPath)
	if path, query, found := strings.Cut(apiPath, "?"); found {
		url = request.JoinURL(c.Endpoint, path) + "?" + query
	}
	rb, err := request.NewJSON(c.RequestInterface, method, url, body)
	if err != nil {
		return err
	}
	return request.DoJSON(
		ctx,
		c.RequestInterface,
		rb,
		out,
		request.WithAction(action),
		request.WithErrorEnvelope(&errorResponse{}),
	)
}
