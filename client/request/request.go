package request

//go:generate mockgen -destination=../mocks/mock_request.go -package=mocks github.com/calypr/data-client/client/request RequestInterface

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/logs"
	"github.com/hashicorp/go-retryablehttp"
)

type Request struct {
	Logs        logs.Logger
	Ctx         context.Context
	RetryClient *retryablehttp.Client
}

type RequestInterface interface {
	New(method, url string) *RequestBuilder
	Do(ctx context.Context, req *RequestBuilder) (*http.Response, error)
}

func NewRequestInterface(
	logger logs.Logger,
	cred *conf.Credential,
) RequestInterface {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.Logger = logger

	baseTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
	}

	authTransport := &AuthTransport{
		Base: baseTransport,
		Cred: cred,
	}

	retryClient.HTTPClient = &http.Client{
		Timeout:   5 * time.Second,
		Transport: authTransport, // The outer shell is now AuthTransport
	}

	return &Request{
		RetryClient: retryClient,
		Logs:        logger,
	}
}

func (r *Request) Do(ctx context.Context, rb *RequestBuilder) (*http.Response, error) {
	// Prepare body reader

	httpReq, err := http.NewRequestWithContext(ctx, rb.Method, rb.Url, rb.Body)
	if err != nil {
		return nil, errors.New("failed to create HTTP request: " + err.Error())
	}

	for key, value := range rb.Headers {
		httpReq.Header.Add(key, value)
	}

	if rb.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+rb.Token)
	}

	// Convert to retryablehttp.Request
	retryReq, err := retryablehttp.FromRequest(httpReq)
	if err != nil {
		return nil, err
	}

	resp, err := r.RetryClient.Do(retryReq)
	if err != nil {
		return resp, errors.New("request failed after retries: " + err.Error())
	}

	return resp, nil
}
