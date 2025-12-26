package client

import (
	"context"
	"fmt"

	"github.com/calypr/data-client/client/api"
	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/logs"
	"github.com/calypr/data-client/client/request"
)

//go:generate mockgen -destination=../mocks/mock_gen3interface.go -package=mocks github.com/calypr/data-client/client/client Gen3Interface

// Top level wrapper Interface for calling lower level interface functions.
//
// Gen3Interface contains minimum number of methods to enable calling functions in the FunctionInterface
// The credential is embedded in the implementation, so it doesn't need to be passed to each method.
type Gen3Interface interface {
	GetCredential() *conf.Credential
	Logger() *logs.TeeLogger

	api.FunctionInterface
}

// Gen3Client wraps jwt.FunctionInterface and embeds the credential
type Gen3Client struct {
	Ctx context.Context
	api.FunctionInterface

	credential *conf.Credential
	logger     *logs.TeeLogger
}

func (g *Gen3Client) Logger() *logs.TeeLogger {
	return g.logger
}

// GetCredential returns the embedded credential
func (g *Gen3Client) GetCredential() *conf.Credential {
	return g.credential
}

// NewGen3Interface returns a Gen3Client that embeds the credential and implements Gen3Interface.
// This eliminates the need to pass credentials around everywhere.
func NewGen3Interface(profile string, logger *logs.TeeLogger, opts ...func(*Gen3Client)) (Gen3Interface, error) {
	config := conf.NewConfigure(logger)
	cred, err := config.Load(profile)
	if err != nil {
		return nil, err
	}

	if valid, err := config.IsValid(cred); !valid {
		return nil, fmt.Errorf("invalid credential: %v", err)
	}

	apiClient := api.NewFunctions(
		config,
		request.NewRequestInterface(logger, cred),
		cred,
		logger,
	)

	return &Gen3Client{
		FunctionInterface: apiClient,
		credential:        cred,
		logger:            logger,
	}, nil
}
