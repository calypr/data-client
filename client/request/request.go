package request

//go:generate mockgen -destination=../mocks/mock_request.go -package=mocks github.com/calypr/data-client/client/request RequestInterface

import (
	"context"
	"errors"
	"net/http"

	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/logs"
	"github.com/hashicorp/go-retryablehttp"
)

type Request struct {
	Logs        logs.Logger
	RetryClient *retryablehttp.Client
}

type RequestInterface interface {
	New(method, url string) *RequestBuilder
	Do(ctx context.Context, req *RequestBuilder) (*http.Response, error)
}

func NewRequestInterface(
	logger logs.Logger,
	cred *conf.Credential,
	conf conf.ManagerInterface,
) RequestInterface {
	baseTransport := &http.Transport{ /* ... your config ... */ }

	authTransport := &AuthTransport{
		Base:    baseTransport,
		Cred:    cred,
		Manager: conf,
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.Logger = logger
	retryClient.HTTPClient.Transport = authTransport

	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		shouldRetry, retryErr :=
			retryablehttp.DefaultRetryPolicy(ctx, resp, err)

		if resp != nil &&
			(resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadGateway) {
			err := authTransport.refreshOnce(ctx)
			if err != nil {
				return false, err
			}
			return true, nil
		}
		return shouldRetry, retryErr
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

	if rb.PartSize != 0 {
		httpReq.ContentLength = rb.PartSize
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
