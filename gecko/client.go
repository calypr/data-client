package gecko

import (
	"context"
	"net/http"
	"strings"

	"github.com/calypr/calypr-cli/conf"
	"github.com/calypr/calypr-cli/githubutil"
	"github.com/calypr/calypr-cli/request"
)

var validateProjectRepoURL = githubutil.ValidateRepositoryURL

type GeckoInterface interface {
	ListConfigTypes(ctx context.Context) ([]ConfigType, error)
	ListConfigs(ctx context.Context, configType ConfigType) ([]string, error)
	GetConfig(ctx context.Context, configType ConfigType, configID string, out any) error
	PutConfig(ctx context.Context, configType ConfigType, configID string, cfg any) (*StatusResponse, error)
	DeleteConfig(ctx context.Context, configType ConfigType, configID string) (*StatusResponse, error)
	GetAppCard(ctx context.Context, projectID string) (*AppCard, error)
	UpsertAppCard(ctx context.Context, projectID string, card AppCard) (*StatusResponse, error)
	DeleteAppCard(ctx context.Context, projectID string) (*StatusResponse, error)
	HealthCheck(ctx context.Context) (string, error)
}

type Client struct {
	request.RequestInterface
	Endpoint string
}

func NewClient(req request.RequestInterface, creds *conf.Credential) GeckoInterface {
	return &Client{
		RequestInterface: req,
		Endpoint:         creds.APIEndpoint,
	}
}

func (c *Client) ListConfigTypes(ctx context.Context) ([]ConfigType, error) {
	var configTypes []ConfigType
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		c.New(http.MethodGet, request.JoinURL(c.Endpoint, "gecko", "types")),
		&configTypes,
		request.WithAction("list config types"),
		request.WithErrorEnvelope(&ErrorResponse{}),
	); err != nil {
		return nil, err
	}
	return configTypes, nil
}

func (c *Client) ListConfigs(ctx context.Context, configType ConfigType) ([]string, error) {
	var configs []string
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		c.New(http.MethodGet, c.configListURL(configType)),
		&configs,
		request.WithAction("list configs"),
		request.WithErrorEnvelope(&ErrorResponse{}),
	); err != nil {
		return nil, err
	}
	return configs, nil
}

func (c *Client) GetConfig(ctx context.Context, configType ConfigType, configID string, out any) error {
	return request.DoJSON(
		ctx,
		c.RequestInterface,
		c.New(http.MethodGet, c.configItemURL(configType, configID)),
		out,
		request.WithAction("get config"),
		request.WithErrorEnvelope(&ErrorResponse{}),
	)
}

func (c *Client) PutConfig(ctx context.Context, configType ConfigType, configID string, cfg any) (*StatusResponse, error) {
	rb, err := request.NewJSON(c.RequestInterface, http.MethodPut, c.configItemURL(configType, configID), cfg)
	if err != nil {
		return nil, err
	}

	var status StatusResponse
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		rb,
		&status,
		request.WithAction("put config"),
		request.WithErrorEnvelope(&ErrorResponse{}),
	); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Client) DeleteConfig(ctx context.Context, configType ConfigType, configID string) (*StatusResponse, error) {
	var status StatusResponse
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		c.New(http.MethodDelete, c.configItemURL(configType, configID)),
		&status,
		request.WithAction("delete config"),
		request.WithErrorEnvelope(&ErrorResponse{}),
	); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Client) GetAppCard(ctx context.Context, projectID string) (*AppCard, error) {
	var card AppCard
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		c.New(http.MethodGet, request.JoinURL(c.Endpoint, "gecko", string(ConfigTypeAppsPage), "appcard", projectID)),
		&card,
		request.WithAction("get app card"),
		request.WithErrorEnvelope(&ErrorResponse{}),
	); err != nil {
		return nil, err
	}
	return &card, nil
}

func (c *Client) UpsertAppCard(ctx context.Context, projectID string, card AppCard) (*StatusResponse, error) {
	rb, err := request.NewJSON(
		c.RequestInterface,
		http.MethodPost,
		request.JoinURL(c.Endpoint, "gecko", string(ConfigTypeAppsPage), "appcard", projectID),
		card,
	)
	if err != nil {
		return nil, err
	}

	var status StatusResponse
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		rb,
		&status,
		request.WithAction("upsert app card"),
		request.WithErrorEnvelope(&ErrorResponse{}),
	); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Client) DeleteAppCard(ctx context.Context, projectID string) (*StatusResponse, error) {
	var status StatusResponse
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		c.New(http.MethodDelete, request.JoinURL(c.Endpoint, "gecko", string(ConfigTypeAppsPage), "appcard", projectID)),
		&status,
		request.WithAction("delete app card"),
		request.WithErrorEnvelope(&ErrorResponse{}),
	); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Client) HealthCheck(ctx context.Context) (string, error) {
	var status string
	if err := request.DoJSON(
		ctx,
		c.RequestInterface,
		c.New(http.MethodGet, request.JoinURL(c.Endpoint, "gecko", "health")),
		&status,
		request.WithAction("health check"),
		request.WithErrorEnvelope(&ErrorResponse{}),
	); err != nil {
		return "", err
	}
	return status, nil
}

func (c *Client) configListURL(configType ConfigType) string {
	if configType == ConfigTypeProjects {
		return request.JoinURL(c.Endpoint, "gecko", "projects", "list")
	}
	return request.JoinURL(c.Endpoint, "gecko", string(configType), "list")
}

func (c *Client) configItemURL(configType ConfigType, configID string) string {
	if configType == ConfigTypeProjects {
		return request.JoinURL(c.Endpoint, append([]string{"gecko", "projects"}, splitConfigID(configID)...)...)
	}
	return request.JoinURL(c.Endpoint, "gecko", string(configType), configID)
}

func splitConfigID(configID string) []string {
	parts := strings.Split(strings.Trim(configID, "/"), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
