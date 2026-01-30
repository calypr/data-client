package requestor

// Request represents a requestor request object
type Request struct {
	RequestID       string   `json:"request_id,omitempty" yaml:"request_id,omitempty"`
	Username        string   `json:"username,omitempty" yaml:"username,omitempty"`
	PolicyID        string   `json:"policy_id,omitempty" yaml:"policy_id,omitempty"`
	ResourcePaths   []string `json:"resource_paths,omitempty" yaml:"resource_paths,omitempty"`
	RoleIDs         []string `json:"role_ids,omitempty" yaml:"role_ids,omitempty"`
	ResourceID      string   `json:"resource_id,omitempty" yaml:"resource_id,omitempty"`
	ResourceDisplay string   `json:"resource_display_name,omitempty" yaml:"resource_display_name,omitempty"`
	Status          string   `json:"status,omitempty" yaml:"status,omitempty"`
	CreatedTime     string   `json:"created_time,omitempty" yaml:"created_time,omitempty"`
	UpdatedTime     string   `json:"updated_time,omitempty" yaml:"updated_time,omitempty"`
	Revoke          bool     `json:"revoke,omitempty" yaml:"revoke,omitempty"`
}

// CreateRequestRequest represents the payload to create a request
type CreateRequestRequest struct {
	Username            string   `json:"username,omitempty" yaml:"username,omitempty"`
	PolicyID            string   `json:"policy_id,omitempty" yaml:"policy_id,omitempty"`
	ResourcePaths       []string `json:"resource_paths,omitempty" yaml:"resource_paths,omitempty"`
	RoleIDs             []string `json:"role_ids,omitempty" yaml:"role_ids,omitempty"`
	ResourceDisplayName string   `json:"resource_display_name,omitempty" yaml:"resource_display_name,omitempty"`
}

// UpdateRequestRequest represents the payload to update a request
type UpdateRequestRequest struct {
	Status string `json:"status" yaml:"status"`
}

type PolicyConfig struct {
	Policies []CreateRequestRequest `yaml:"policies"`
}
