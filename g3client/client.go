package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/fence"
	"github.com/calypr/data-client/indexd"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
	version "github.com/hashicorp/go-version"
)

//go:generate mockgen -destination=../mocks/mock_gen3interface.go -package=mocks github.com/calypr/data-client/g3client Gen3Interface

type Gen3Interface interface {
	GetCredential() *conf.Credential
	Logger() *logs.Gen3Logger
	ExportCredential(ctx context.Context, cred *conf.Credential) error
	Fence() fence.FenceInterface
	Indexd() indexd.IndexdInterface
}

func NewGen3InterfaceFromCredential(cred *conf.Credential, logger *logs.Gen3Logger) Gen3Interface {
	config := conf.NewConfigure(logger.Logger)
	reqInterface := request.NewRequestInterface(logger.Logger, cred, config)
	fClient := fence.NewFenceClient(reqInterface, cred, logger.Logger)
	iClient := indexd.NewIndexdClient(reqInterface, cred, logger.Logger)

	return &Gen3Client{
		fence:            fClient,
		indexd:           iClient,
		config:           config,
		RequestInterface: reqInterface,
		credential:       cred,
		logger:           logger,
	}
}

type Gen3Client struct {
	Ctx    context.Context
	fence  fence.FenceInterface
	indexd indexd.IndexdInterface
	config conf.ManagerInterface
	request.RequestInterface

	credential *conf.Credential
	logger     *logs.Gen3Logger
}

func (g *Gen3Client) Fence() fence.FenceInterface {
	return g.fence
}

func (g *Gen3Client) Indexd() indexd.IndexdInterface {
	return g.indexd
}

func (g *Gen3Client) Logger() *logs.Gen3Logger {
	return g.logger
}

func (g *Gen3Client) GetCredential() *conf.Credential {
	return g.credential
}

func (g *Gen3Client) ExportCredential(ctx context.Context, cred *conf.Credential) error {
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

// NewGen3Interface returns a Gen3Client that embeds the credential and implements Gen3Interface.
func NewGen3Interface(profile string, logger *logs.Gen3Logger, opts ...func(*Gen3Client)) (Gen3Interface, error) {
	config := conf.NewConfigure(logger.Logger)
	cred, err := config.Load(profile)
	if err != nil {
		return nil, err
	}

	if valid, err := config.IsValid(cred); !valid {
		return nil, fmt.Errorf("invalid credential: %v", err)
	}

	reqInterface := request.NewRequestInterface(logger.Logger, cred, config)
	fClient := fence.NewFenceClient(reqInterface, cred, logger.Logger)
	iClient := indexd.NewIndexdClient(reqInterface, cred, logger.Logger)

	return &Gen3Client{
		fence:            fClient,
		indexd:           iClient,
		config:           config,
		RequestInterface: reqInterface,
		credential:       cred,
		logger:           logger,
	}, nil
}
