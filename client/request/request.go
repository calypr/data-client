package req

//go:generate mockgen -destination=../mocks/mock_request.go -package=mocks github.com/calypr/data-client/client/request RequestInterface

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/calypr/data-client/client/common"
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
	MakeARequest(method string, apiEndpoint string, accessToken string, contentType string, headers map[string]string, body *bytes.Buffer, noTimeout bool) (*http.Response, error)
	RequestNewAccessToken(accessTokenEndpoint string, profileConfig *conf.Credential) error
	Logger() logs.Logger
}

func (r *Request) Logger() logs.Logger {
	return r.Logs
}

func NewRequest(ctx context.Context, logger logs.Logger) *Request {
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

func (r *Request) MakeARequest(method string, apiEndpoint string, accessToken string, contentType string, headers map[string]string, body *bytes.Buffer, noTimeout bool) (*http.Response, error) {
	/*
	   Make http request with header and body
	*/
	if headers == nil {
		headers = make(map[string]string)
	}
	if accessToken != "" {
		headers["Authorization"] = "Bearer " + accessToken
	}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	var client *http.Client
	if noTimeout {
		client = &http.Client{}
	} else {
		client = &http.Client{Timeout: common.DefaultTimeout}
	}
	var req *http.Request
	var err error
	if body == nil {
		req, err = http.NewRequestWithContext(r.Ctx, method, apiEndpoint, nil)
	} else {
		req, err = http.NewRequestWithContext(r.Ctx, method, apiEndpoint, body)
	}
	if err != nil {
		return nil, errors.New("Error occurred during generating HTTP request: " + err.Error())
	}
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("Error occurred during making HTTP request: " + err.Error())
	}
	return resp, nil
}

func (r *Request) RequestNewAccessToken(accessTokenEndpoint string, profileConfig *conf.Credential) error {
	/*
		Request new access token to replace the expired one.

		Args:
			accessTokenEndpoint: the api endpoint for request new access token
		Returns:
			profileConfig: new credential
			err: error

	*/
	body := bytes.NewBufferString("{\"api_key\": \"" + profileConfig.APIKey + "\"}")
	resp, err := r.MakeARequest("POST", accessTokenEndpoint, "", "application/json", nil, body, false)
	var m common.AccessTokenStruct
	// parse resp error codes first for profile configuration verification
	if resp != nil && resp.StatusCode != 200 {
		return errors.New("Error occurred in RequestNewAccessToken with error code " + strconv.Itoa(resp.StatusCode) + ", check FENCE log for more details.")
	}
	if err != nil {
		return errors.New("Error occurred in RequestNewAccessToken: " + err.Error())
	}
	defer resp.Body.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	respStr := buf.String()

	err = json.Unmarshal([]byte(respStr), &m)
	if err != nil {
		return errors.New("Error occurred in RequestNewAccessToken: " + err.Error())
	}

	if m.AccessToken == "" {
		return errors.New("Could not get new access key from response string: " + respStr)
	}
	profileConfig.AccessToken = m.AccessToken
	return nil
}
