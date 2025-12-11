package client

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/calypr/data-client/client/jwt"
	"github.com/calypr/data-client/client/logs"
)

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

	Logger() logs.Logger
}

// Gen3Client wraps jwt.FunctionInterface and embeds the credential
type Gen3Client struct {
	jwt.FunctionInterface
	credential *jwt.Credential
	logger     logs.Logger
}

func (g *Gen3Client) Logger() logs.Logger {
	if g.logger == nil {
		return logs.Default() // fallback to standard log
	}
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
	request := &jwt.Request{}
	return request.MakeARequest(method, apiEndpoint, accessToken, contentType, headers, body, noTimeout)
}

// DeleteRecord deletes a record from INDEXD as well as its storage locations
func (g *Gen3Client) DeleteRecord(guid string) (string, error) {
	// Use the embedded credential
	// Since DeleteRecord is not part of FunctionInterface, we need to access it via type assertion
	// or create a new Functions instance. We'll use type assertion first.
	if functions, ok := g.FunctionInterface.(*jwt.Functions); ok {
		return functions.DeleteRecord(g.credential, guid)
	}
	// Fallback: create a new Functions instance if type assertion fails
	config := &jwt.Configure{}
	request := &jwt.Request{}
	functionInterface := jwt.NewFunctions(config, request)
	// Cast to *Functions to access DeleteRecord
	if functions, ok := functionInterface.(*jwt.Functions); ok {
		return functions.DeleteRecord(g.credential, guid)
	}
	// This should never happen, but handle it gracefully
	return "", errors.New("unable to access DeleteRecord method")
}

func NewGen3Interface(profile string) (Gen3Interface, error) {
	return NewGen3InterfaceWithLogger(profile, nil)
}

// NewGen3Interface returns a Gen3Client that embeds the credential and implements Gen3Interface.
// This eliminates the need to pass credentials around everywhere.
func NewGen3InterfaceWithLogger(profile string, logger logs.Logger) (Gen3Interface, error) {
	config := &jwt.Configure{}
	request := &jwt.Request{}
	client := jwt.NewFunctions(config, request)

	cred, err := config.ParseConfig(profile)
	if err != nil {
		return nil, err
	}
	if valid, err := config.IsValidCredential(cred); !valid {
		return nil, fmt.Errorf("invalid credential: %v", err)
	}

	if logger == nil {
		logger = logs.Default() // e.g. standard log wrapper
	}

	return &Gen3Client{
		FunctionInterface: client,
		credential:        &cred,
		logger:            logger,
	}, nil
}
