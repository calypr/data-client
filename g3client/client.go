package g3client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/fence"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
	"github.com/calypr/data-client/requestor"
	"github.com/calypr/data-client/sower"
	"github.com/calypr/syfon/client/credentials"
	version "github.com/hashicorp/go-version"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=../mocks/mock_gen3interface.go -package=mocks github.com/calypr/data-client/g3client Gen3Interface

type Gen3Interface interface {
	request.RequestInterface
	Logger() *logs.Gen3Logger
	Credentials() credentials.Manager
	SyfonClient() SyfonClientInterface
	FenceClient() fence.FenceInterface
	RequestorClient() requestor.RequestorInterface
	SowerClient() sower.SowerInterface
}

func NewGen3InterfaceFromCredential(cred *conf.Credential, logger *logs.Gen3Logger, opts ...Option) Gen3Interface {
	config := conf.NewConfigure(logger.Logger)
	reqInterface := request.NewRequestInterface(logger, cred, config)

	client := &Gen3Client{
		config:           config,
		RequestInterface: reqInterface,
		credential:       cred,
		logger:           logger,
	}

	for _, opt := range opts {
		opt(client)
	}

	client.initializeClients()

	return client
}

func (g *Gen3Client) initializeClients() {
	shouldInit := func(ct ClientType) bool {
		if len(g.requestedClients) == 0 {
			return true
		}
		for _, c := range g.requestedClients {
			if c == ct {
				return true
			}
		}
		return false
	}

	if shouldInit(FenceClient) {
		g.fence = fence.NewFenceClient(g.RequestInterface, g.credential, g.logger.Logger)
	}
	if shouldInit(SyfonClient) {
		g.syfon = buildSyfonClient(g.credential, g.logger, g.RequestInterface)
	}
	if shouldInit(SowerClient) {
		g.sower = sower.NewSowerClient(g.RequestInterface, g.credential.APIEndpoint)
	}
	if shouldInit(RequestorClient) {
		g.requestor = requestor.NewRequestorClient(g.RequestInterface, g.credential)
	}
}

type Gen3Client struct {
	Ctx       context.Context
	fence     fence.FenceInterface
	syfon     SyfonClientInterface
	sower     sower.SowerInterface
	requestor requestor.RequestorInterface
	config    conf.ManagerInterface
	request.RequestInterface

	credential *conf.Credential
	creds      credentials.Manager
	logger     *logs.Gen3Logger

	requestedClients []ClientType
}

type ClientType string

const (
	FenceClient     ClientType = "fence"
	SyfonClient     ClientType = "syfon"
	SowerClient     ClientType = "sower"
	RequestorClient ClientType = "requestor"
)

type Option func(*Gen3Client)

func WithClients(clients ...ClientType) Option {
	return func(g *Gen3Client) {
		g.requestedClients = clients
	}
}

func (g *Gen3Client) SyfonClient() SyfonClientInterface {
	if g.syfon == nil {
		g.syfon = buildSyfonClient(g.credential, g.logger, g.RequestInterface)
	}
	return g.syfon
}

func (g *Gen3Client) FenceClient() fence.FenceInterface {
	return g.fence
}

func (g *Gen3Client) RequestorClient() requestor.RequestorInterface {
	return g.requestor
}

func (g *Gen3Client) SowerClient() sower.SowerInterface {
	return g.sower
}

func (g *Gen3Client) exportCredential(ctx context.Context, cred *conf.Credential) error {
	if cred.Profile == "" {
		return fmt.Errorf("profile name is required")
	}
	if cred.APIEndpoint == "" {
		return fmt.Errorf("API endpoint is required")
	}

	// Normalize endpoint
	cred.APIEndpoint = strings.TrimSpace(cred.APIEndpoint)
	cred.APIEndpoint = strings.TrimSuffix(cred.APIEndpoint, "/")

	// Validate URL format
	parsedURL, err := conf.ValidateUrl(cred.APIEndpoint)
	if err != nil {
		return fmt.Errorf("invalid apiendpoint URL: %w", err)
	}
	fenceBase := parsedURL.Scheme + "://" + parsedURL.Host
	if _, err := g.config.Load(cred.Profile); err != nil && !errors.Is(err, conf.ErrProfileNotFound) {
		return err
	}

	if cred.APIKey != "" {
		// Always refresh the access token — ignore any old one that might be in the struct
		token, err := g.fence.NewAccessToken(ctx)
		if err != nil {
			if strings.Contains(err.Error(), "401") {
				return fmt.Errorf("authentication failed (401) for %s — your API key is invalid, revoked, or expired", fenceBase)
			}
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "no such host") {
				return fmt.Errorf("cannot reach Fence at %s — is this a valid Gen3 commons?", fenceBase)
			}
			return fmt.Errorf("failed to refresh access token: %w", err)
		}
		g.credential.AccessToken = token
	} else {
		g.logger.Warn("WARNING: Your profile will only be valid for 24 hours since you have only provided a refresh token for authentication")
	}

	// Clean up shepherd flags
	cred.UseShepherd = strings.TrimSpace(cred.UseShepherd)
	cred.MinShepherdVersion = strings.TrimSpace(cred.MinShepherdVersion)

	if cred.MinShepherdVersion != "" {
		if _, err = version.NewVersion(cred.MinShepherdVersion); err != nil {
			return fmt.Errorf("invalid min-shepherd-version: %w", err)
		}
	}

	if err := g.config.Save(cred); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

type gen3Credentials struct {
	client *Gen3Client
}

func (c *gen3Credentials) Current() *conf.Credential {
	return c.client.credential
}

func (c *gen3Credentials) Export(ctx context.Context, cred *conf.Credential) error {
	return c.client.exportCredential(ctx, cred)
}

func (g *Gen3Client) Credentials() credentials.Manager {
	if g.creds == nil {
		g.creds = &gen3Credentials{client: g}
	}
	return g.creds
}

// EnsureValidCredential checks if the credential is valid and refreshes it if the access token is expired but the API key is valid.
// It accepts an optional fClient; if nil, it will initialize one internally if needed for refresh.
func EnsureValidCredential(ctx context.Context, cred *conf.Credential, config conf.ManagerInterface, logger *logs.Gen3Logger, fClient fence.FenceInterface) error {
	if valid, err := config.IsCredentialValid(cred); !valid {
		if strings.Contains(err.Error(), "access_token is invalid but api_key is valid") {
			// Try to refresh the token
			if fClient == nil {
				reqInterface := request.NewRequestInterface(logger, cred, config)
				fClient = fence.NewFenceClient(reqInterface, cred, logger.Logger)
			}
			newToken, refreshErr := fClient.NewAccessToken(ctx)
			if refreshErr == nil {
				cred.AccessToken = newToken
				err = config.Save(cred)
				if err != nil {
					logger.Warn(fmt.Sprintf("Failed to save refreshed token: %v", err))
				}
				return nil
			}
			return fmt.Errorf("failed to refresh access token: %v (original error: %v)", refreshErr, err)
		}
		return fmt.Errorf("invalid credential: %v", err)
	}
	return nil
}

// NewGen3Interface returns a Gen3Client that embeds the credential and implements Gen3Interface.
func NewGen3Interface(profile string, logger *logs.Gen3Logger, opts ...Option) (Gen3Interface, error) {
	config := conf.NewConfigure(logger.Logger)
	cred, err := config.Load(profile)
	if err != nil {
		return nil, err
	}

	reqInterface := request.NewRequestInterface(logger, cred, config)

	// We need a temporary Fence client to refresh tokens if needed
	fClient := fence.NewFenceClient(reqInterface, cred, logger.Logger)
	if err := EnsureValidCredential(context.Background(), cred, config, logger, fClient); err != nil {
		return nil, err
	}

	client := &Gen3Client{
		config:           config,
		RequestInterface: reqInterface,
		credential:       cred,
		logger:           logger,
	}

	for _, opt := range opts {
		opt(client)
	}

	client.initializeClients()

	return client, nil
}
func (g *Gen3Client) Logger() *logs.Gen3Logger { return g.logger }
