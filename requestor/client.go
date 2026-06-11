package requestor

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/request"
	"gopkg.in/yaml.v3"
)

//go:embed policies/*.yaml
var policyFS embed.FS

type RequestorClient struct {
	request.RequestInterface
	Endpoint string
}

func NewRequestorClient(req request.RequestInterface, creds *conf.Credential) *RequestorClient {
	return &RequestorClient{
		RequestInterface: req,
		Endpoint:         creds.APIEndpoint,
	}
}

// Ensure interface compliance
var _ RequestorInterface = &RequestorClient{}

type RequestorInterface interface {
	ListRequests(ctx context.Context, mine bool, active bool, username string) ([]Request, error)
	CreateRequest(ctx context.Context, req CreateRequestRequest, revoke bool) (*Request, error)
	UpdateRequest(ctx context.Context, requestID string, status string) (*Request, error)
	AddUser(ctx context.Context, projectID string, username string, write bool, guppy bool) ([]Request, error)
	AddUserToResources(ctx context.Context, resources []ProjectResource, username string, write bool, guppy bool) ([]Request, error)
	RemoveUser(ctx context.Context, projectID string, username string) ([]Request, error)
}

func (c *RequestorClient) ListRequests(ctx context.Context, mine bool, active bool, username string) ([]Request, error) {
	url := request.JoinURL(c.Endpoint, "requestor", "request")
	if mine {
		url = request.JoinURL(c.Endpoint, "requestor", "request", "user")
	}

	params := []string{}
	if active {
		params = append(params, "active")
	}
	if username != "" && !mine {
		params = append(params, fmt.Sprintf("username=%s", username))
	}

	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	var requests []Request
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		c.New(http.MethodGet, url),
		&requests,
		request.WithAction("failed to list requests"),
		request.WithExpectedStatus(http.StatusOK),
	); err != nil {
		return nil, err
	}
	return requests, nil
}

func (c *RequestorClient) CreateRequest(ctx context.Context, reqPayload CreateRequestRequest, revoke bool) (*Request, error) {
	url := request.JoinURL(c.Endpoint, "requestor", "request")
	if revoke {
		url += "?revoke"
	}

	rb, err := request.NewJSON(c.RequestInterface, http.MethodPost, url, reqPayload)
	if err != nil {
		return nil, err
	}

	var createdRequest Request
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		rb,
		&createdRequest,
		request.WithAction("failed to create request"),
	); err != nil {
		return nil, err
	}
	return &createdRequest, nil
}

func (c *RequestorClient) UpdateRequest(ctx context.Context, requestID string, status string) (*Request, error) {
	url := request.JoinURL(c.Endpoint, "requestor", "request", requestID)
	payload := UpdateRequestRequest{Status: status}

	rb, err := request.NewJSON(c.RequestInterface, http.MethodPut, url, payload)
	if err != nil {
		return nil, err
	}

	var updatedRequest Request
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		rb,
		&updatedRequest,
		request.WithAction("failed to update request"),
	); err != nil {
		return nil, err
	}
	return &updatedRequest, nil
}

func loadPolicies(filename string) ([]CreateRequestRequest, error) {
	content, err := policyFS.ReadFile("policies/" + filename)
	if err != nil {
		return nil, err
	}

	var config PolicyConfig
	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil, err
	}
	return config.Policies, nil
}

func formatPolicy(policy CreateRequestRequest, projectID string, username string) CreateRequestRequest {
	p := policy
	if username != "" {
		p.Username = username
	}

	if projectID != "" {
		parts := strings.SplitN(projectID, "-", 2)
		if len(parts) == 2 {
			program := parts[0]
			project := parts[1]

			newPaths := make([]string, len(p.ResourcePaths))
			for i, path := range p.ResourcePaths {
				r := strings.ReplaceAll(path, "PROGRAM", program)
				r = strings.ReplaceAll(r, "PROJECT", project)
				newPaths[i] = r
			}
			p.ResourcePaths = newPaths
		}
		p.ResourceDisplayName = projectID
	}
	return p
}

func formatPolicyForResource(policy CreateRequestRequest, resource ProjectResource, username string) CreateRequestRequest {
	p := policy
	if username != "" {
		p.Username = username
	}

	if resource.ResourcePath != "" {
		newPaths := make([]string, len(p.ResourcePaths))
		for i, path := range p.ResourcePaths {
			if strings.Contains(path, "PROGRAM") || strings.Contains(path, "PROJECT") {
				newPaths[i] = resource.ResourcePath
				continue
			}
			newPaths[i] = path
		}
		p.ResourcePaths = newPaths
		p.ResourceDisplayName = resource.DisplayName()
	}
	return p
}

func ParseProjectResources(raw string) ([]ProjectResource, error) {
	raw = extractProjectResourceList(raw)
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})

	seen := map[string]struct{}{}
	resources := make([]ProjectResource, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.Trim(part, `"'`))
		if trimmed == "" || strings.HasPrefix(trimmed, "(") {
			continue
		}
		resource, err := ParseProjectResource(part)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[resource.ResourcePath]; ok {
			continue
		}
		seen[resource.ResourcePath] = struct{}{}
		resources = append(resources, resource)
	}
	if len(resources) == 0 {
		return nil, fmt.Errorf("no project resources found")
	}
	return resources, nil
}

func extractProjectResourceList(raw string) string {
	markers := []string{
		"copy/paste scope list:",
		"missing organization/project scopes:",
		"denied organization/project scopes:",
		"denied resource paths:",
	}
	lower := strings.ToLower(raw)
	start := -1
	markerLen := 0
	for _, marker := range markers {
		if idx := strings.LastIndex(lower, marker); idx >= 0 && idx >= start {
			start = idx
			markerLen = len(marker)
		}
	}
	if start >= 0 {
		raw = raw[start+markerLen:]
	}
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "\r\n\r\n"); idx >= 0 {
		raw = raw[:idx]
	}
	if idx := strings.Index(raw, "\n\n"); idx >= 0 {
		raw = raw[:idx]
	}
	return raw
}

func ParseProjectResource(raw string) (ProjectResource, error) {
	candidate := strings.TrimSpace(raw)
	candidate = strings.Trim(candidate, `"'`)
	if idx := strings.Index(candidate, "/programs/"); idx >= 0 {
		candidate = candidate[idx:]
	} else if idx := strings.LastIndex(candidate, ":"); idx >= 0 {
		candidate = candidate[idx+1:]
	}
	candidate = strings.TrimSpace(candidate)
	candidate = strings.Trim(candidate, `"'`)
	candidate = strings.TrimSuffix(candidate, ".")
	if candidate == "" || strings.HasPrefix(candidate, "(") {
		return ProjectResource{}, fmt.Errorf("empty resource path")
	}

	if parsed, err := url.Parse(candidate); err == nil && parsed.Path != "" {
		candidate = parsed.Path
	}

	trimmed := strings.Trim(candidate, "/")
	if !strings.HasPrefix(trimmed, "programs/") && strings.Count(trimmed, "/") == 1 {
		pair := strings.SplitN(trimmed, "/", 2)
		return projectResource(pair[0], pair[1])
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) < 4 || parts[0] != "programs" || parts[2] != "projects" {
		return ProjectResource{}, fmt.Errorf("invalid project resource %q; expected /programs/<organization>/projects/<project>", raw)
	}
	return projectResource(parts[1], parts[3])
}

func projectResource(program, project string) (ProjectResource, error) {
	program = strings.TrimSpace(program)
	project = strings.TrimSpace(project)
	if program == "" || project == "" {
		return ProjectResource{}, fmt.Errorf("project resource requires both organization and project")
	}
	if strings.ContainsAny(program, " \t") || strings.ContainsAny(project, " \t") {
		return ProjectResource{}, fmt.Errorf("project resource contains whitespace: %s/%s", program, project)
	}
	return ProjectResource{
		Program:      program,
		Project:      project,
		ResourcePath: "/programs/" + program + "/projects/" + project,
	}, nil
}

func (c *RequestorClient) getPolicyKey(p CreateRequestRequest) string {
	roles := make([]string, len(p.RoleIDs))
	copy(roles, p.RoleIDs)
	sort.Strings(roles)

	paths := make([]string, len(p.ResourcePaths))
	copy(paths, p.ResourcePaths)
	sort.Strings(paths)

	return fmt.Sprintf("%s:%s:%s", p.PolicyID, strings.Join(roles, ","), strings.Join(paths, ","))
}

func (c *RequestorClient) AddUser(ctx context.Context, projectID string, username string, write bool, guppy bool) ([]Request, error) {
	uniquePolicies := make(map[string]CreateRequestRequest)

	addFrom := func(fileName string) error {
		pols, err := loadPolicies(fileName)
		if err != nil {
			return err
		}
		for _, p := range pols {
			formatted := formatPolicy(p, projectID, username)
			key := c.getPolicyKey(formatted)
			uniquePolicies[key] = formatted
		}
		return nil
	}

	// Always add read
	if err := addFrom("add-user-read.yaml"); err != nil {
		return nil, fmt.Errorf("failed to load read policy: %w", err)
	}

	if write {
		if err := addFrom("add-user-write.yaml"); err != nil {
			return nil, fmt.Errorf("failed to load write policy: %w", err)
		}
	}
	if guppy {
		if err := addFrom("add-user-guppy-admin.yaml"); err != nil {
			return nil, fmt.Errorf("failed to load guppy policy: %w", err)
		}
	}

	var createdRequests []Request
	for _, formatted := range uniquePolicies {
		req, err := c.CreateRequest(ctx, formatted, false)
		if err != nil {
			return createdRequests, fmt.Errorf("failed to create request for policy %v: %w", formatted, err)
		}
		createdRequests = append(createdRequests, *req)
	}
	return createdRequests, nil
}

func (c *RequestorClient) AddUserToResources(ctx context.Context, resources []ProjectResource, username string, write bool, guppy bool) ([]Request, error) {
	uniquePolicies := make(map[string]CreateRequestRequest)

	addFrom := func(fileName string, resource ProjectResource) error {
		pols, err := loadPolicies(fileName)
		if err != nil {
			return err
		}
		for _, p := range pols {
			formatted := formatPolicyForResource(p, resource, username)
			key := c.getPolicyKey(formatted)
			uniquePolicies[key] = formatted
		}
		return nil
	}

	for _, resource := range resources {
		if err := addFrom("add-user-read.yaml", resource); err != nil {
			return nil, fmt.Errorf("failed to load read policy for %s: %w", resource.DisplayName(), err)
		}
		if write {
			if err := addFrom("add-user-write.yaml", resource); err != nil {
				return nil, fmt.Errorf("failed to load write policy for %s: %w", resource.DisplayName(), err)
			}
		}
		if guppy {
			if err := addFrom("add-user-guppy-admin.yaml", resource); err != nil {
				return nil, fmt.Errorf("failed to load guppy policy for %s: %w", resource.DisplayName(), err)
			}
		}
	}

	var createdRequests []Request
	for _, formatted := range uniquePolicies {
		req, err := c.CreateRequest(ctx, formatted, false)
		if err != nil {
			return createdRequests, fmt.Errorf("failed to create request for policy %v: %w", formatted, err)
		}
		createdRequests = append(createdRequests, *req)
	}
	return createdRequests, nil
}

func (c *RequestorClient) RemoveUser(ctx context.Context, projectID string, username string) ([]Request, error) {
	uniquePolicies := make(map[string]CreateRequestRequest)

	addFrom := func(fileName string) error {
		pols, err := loadPolicies(fileName)
		if err != nil {
			return err
		}
		for _, p := range pols {
			formatted := formatPolicy(p, projectID, username)
			key := c.getPolicyKey(formatted)
			uniquePolicies[key] = formatted
		}
		return nil
	}

	// Revoke read and write
	if err := addFrom("add-user-read.yaml"); err != nil {
		return nil, fmt.Errorf("failed to load read policy: %w", err)
	}

	if err := addFrom("add-user-write.yaml"); err != nil {
		return nil, fmt.Errorf("failed to load write policy: %w", err)
	}

	var createdRequests []Request
	for _, formatted := range uniquePolicies {
		req, err := c.CreateRequest(ctx, formatted, true) // revoke=true
		if err != nil {
			return createdRequests, fmt.Errorf("failed to revoke request: %w", err)
		}
		createdRequests = append(createdRequests, *req)
	}
	return createdRequests, nil
}
