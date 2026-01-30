package requestor

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	RemoveUser(ctx context.Context, projectID string, username string) ([]Request, error)
}

func (c *RequestorClient) ListRequests(ctx context.Context, mine bool, active bool, username string) ([]Request, error) {
	url := c.Endpoint + "/requestor/request"
	if mine {
		url += "/user"
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

	rb := c.New(http.MethodGet, url)
	resp, err := c.Do(ctx, rb)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list requests: status %d", resp.StatusCode)
	}

	var requests []Request
	if err := json.NewDecoder(resp.Body).Decode(&requests); err != nil {
		return nil, err
	}
	return requests, nil
}

func (c *RequestorClient) CreateRequest(ctx context.Context, reqPayload CreateRequestRequest, revoke bool) (*Request, error) {
	url := c.Endpoint + "/requestor/request"
	if revoke {
		url += "?revoke"
	}

	rb := c.New(http.MethodPost, url)
	rb, err := rb.WithJSONBody(reqPayload)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(ctx, rb)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create request: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var createdRequest Request
	if err := json.NewDecoder(resp.Body).Decode(&createdRequest); err != nil {
		return nil, err
	}
	return &createdRequest, nil
}

func (c *RequestorClient) UpdateRequest(ctx context.Context, requestID string, status string) (*Request, error) {
	url := fmt.Sprintf("%s/requestor/request/%s", c.Endpoint, requestID)
	payload := UpdateRequestRequest{Status: status}

	rb := c.New(http.MethodPut, url)
	rb, err := rb.WithJSONBody(payload)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(ctx, rb)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to update request: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var updatedRequest Request
	if err := json.NewDecoder(resp.Body).Decode(&updatedRequest); err != nil {
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
		parts := strings.Split(projectID, "-")
		if len(parts) >= 2 {
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
