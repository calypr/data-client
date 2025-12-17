package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/calypr/data-client/client/jwt"
	"github.com/calypr/data-client/client/logs"
)

//go:generate mockgen -destination=../mocks/mock_gen3interface.go -package=mocks github.com/calypr/data-client/client/gen3Client Gen3Interface

// Gen3Interface contains methods used to make authorized http requests to Gen3 services.
// The credential is embedded in the implementation, so it doesn't need to be passed to each method.
type Gen3Interface interface {
	CheckPrivileges() (string, map[string]any, error)
	CheckForShepherdAPI() (bool, error)
	GetResponse(endpointPostPrefix string, method string, contentType string, bodyBytes []byte) (string, *http.Response, error)
	DoRequestWithSignedHeader(endpointPostPrefix string, contentType string, bodyBytes []byte) (jwt.JsonMessage, error)
	MakeARequest(method string, apiEndpoint string, accessToken string, contentType string, headers map[string]string, body *bytes.Buffer, noTimeout bool) (*http.Response, error)
	GetHost() (*url.URL, error)
	GetCredential() *jwt.Credential
	DeleteRecord(guid string) (string, error)

	Logger() *logs.TeeLogger
}

// Gen3Client wraps jwt.FunctionInterface and embeds the credential
type Gen3Client struct {
	Ctx               context.Context
	FunctionInterface jwt.FunctionInterface
	credential        *jwt.Credential

	logger *logs.TeeLogger
}

func (g *Gen3Client) Logger() *logs.TeeLogger {
	return g.logger
}

// CheckPrivileges wraps the underlying method with embedded credential
func (g *Gen3Client) CheckPrivileges() (string, map[string]any, error) {
	return g.FunctionInterface.CheckPrivileges(g.credential)
}

// CheckForShepherdAPI wraps the underlying method with embedded credential
func (g *Gen3Client) CheckForShepherdAPI() (bool, error) {
	return g.FunctionInterface.CheckForShepherdAPI(g.credential)
}

// GetResponse wraps the underlying method with embedded credential
func (g *Gen3Client) GetResponse(endpointPostPrefix string, method string, contentType string, bodyBytes []byte) (string, *http.Response, error) {
	return g.FunctionInterface.GetResponse(g.credential, endpointPostPrefix, method, contentType, bodyBytes)
}

// DoRequestWithSignedHeader wraps the underlying method with embedded credential
func (g *Gen3Client) DoRequestWithSignedHeader(endpointPostPrefix string, contentType string, bodyBytes []byte) (jwt.JsonMessage, error) {
	return g.FunctionInterface.DoRequestWithSignedHeader(g.credential, endpointPostPrefix, contentType, bodyBytes)
}

// GetHost wraps the underlying method with embedded credential
func (g *Gen3Client) GetHost() (*url.URL, error) {
	return g.FunctionInterface.GetHost(g.credential)
}

// GetCredential returns the embedded credential
func (g *Gen3Client) GetCredential() *jwt.Credential {
	return g.credential
}

// MakeARequest wraps the underlying Request.MakeARequest method
func (g *Gen3Client) MakeARequest(method string, apiEndpoint string, accessToken string, contentType string, headers map[string]string, body *bytes.Buffer, noTimeout bool) (*http.Response, error) {
	// Access the underlying Request through the Functions struct
	// We need to create a temporary Request instance since we can't access it directly
	if functions, ok := g.FunctionInterface.(*jwt.Functions); ok {
		return functions.Request.MakeARequest(method, apiEndpoint, accessToken, contentType, headers, body, noTimeout)
	}
	return nil, errors.New("unable to access MakeARequest method")
}

// DeleteRecord deletes a record from INDEXD as well as its storage locations
func (g *Gen3Client) DeleteRecord(guid string) (string, error) {
	// Use the embedded credential
	// Since DeleteRecord is not part of FunctionInterface, we need to access it via type assertion
	// or create a new Functions instance. We'll use type assertion first.
	if functions, ok := g.FunctionInterface.(*jwt.Functions); ok {
		return functions.DeleteRecord(g.credential, guid)
	}

	// This should never happen, but handle it gracefully
	return "", errors.New("unable to access DeleteRecord method")
}

// NewGen3Interface returns a Gen3Client that embeds the credential and implements Gen3Interface.
// This eliminates the need to pass credentials around everywhere.
func NewGen3Interface(ctx context.Context, profile string, logger *logs.TeeLogger, opts ...func(*Gen3Client)) (Gen3Interface, error) {
	// Note: A tee logger must be passed here otherwise you risk causing panics.

	config := &jwt.Configure{}
	request := &jwt.Request{Ctx: ctx, Logs: logger}
	client := jwt.NewFunctions(ctx, config, request)

	cred, err := config.ParseConfig(profile)
	if err != nil {
		return nil, err
	}
	if valid, err := config.IsValidCredential(cred); !valid {
		return nil, fmt.Errorf("invalid credential: %v", err)
	}

	return &Gen3Client{
		FunctionInterface: client,
		credential:        &cred,
		logger:            logger,
	}, nil
}
