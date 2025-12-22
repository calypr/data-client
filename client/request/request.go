package req

//go:generate mockgen -destination=../mocks/mock_request.go -package=mocks github.com/calypr/data-client/client/request RequestInterface

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	Do(req *RequestBuilder) (*http.Response, error)
	DoAuthenticated(rb *RequestBuilder, cred *conf.Credential, refreshToken func(*conf.Credential) error) (*http.Response, error)
}

func NewRequestInterface(ctx context.Context, logger logs.Logger) RequestInterface {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 10
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 30 * time.Second
	retryClient.Backoff = retryablehttp.DefaultBackoff
	retryClient.CheckRetry = retryablehttp.DefaultRetryPolicy
	retryClient.Logger = nil

	retryClient.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	return &Request{
		Ctx:         ctx,
		RetryClient: retryClient,
		Logs:        logger,
	}
}

func (r *Request) Do(rb *RequestBuilder) (*http.Response, error) {
	// Prepare body reader
	var bodyReader *bytes.Buffer
	if len(rb.Body) > 0 {
		bodyReader = bytes.NewBuffer(rb.Body)
	}

	httpReq, err := http.NewRequestWithContext(r.Ctx, rb.Method, rb.Url, bodyReader)
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

	oldTimeout := r.RetryClient.HTTPClient.Timeout
	if rb.Timeout == false {
		r.RetryClient.HTTPClient.Timeout = 0
		defer func() { r.RetryClient.HTTPClient.Timeout = oldTimeout }()
	}

	resp, err := r.RetryClient.Do(retryReq)
	if err != nil {
		return nil, errors.New("request failed after retries: " + err.Error())
	}

	return resp, nil
}

func (r *Request) DoAuthenticated(rb *RequestBuilder, cred *conf.Credential, refreshToken func(*conf.Credential) error) (*http.Response, error) {
	// First attempt with current token (if any)
	if cred.AccessToken != "" {
		rb = rb.WithToken(cred.AccessToken)
	}

	resp, err := r.Do(rb)
	if err != nil {
		return resp, err
	}

	// Only attempt refresh+retry if we got 401/503 AND we have a way to refresh
	if (resp.StatusCode == 401 || resp.StatusCode == 503) && cred.APIKey != "" {
		resp.Body.Close()

		if err := refreshToken(cred); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}

		// Retry once with new token
		rb = rb.WithToken(cred.AccessToken)
		return r.Do(rb)
	}

	return resp, nil
}
