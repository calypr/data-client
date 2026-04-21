package g3client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
	"github.com/calypr/syfon/apigen/client/bucketapi"
	"github.com/calypr/syfon/apigen/client/drs"
	"github.com/calypr/syfon/apigen/client/internalapi"
	"github.com/calypr/syfon/apigen/client/lfsapi"
	"github.com/calypr/syfon/apigen/client/metricsapi"
	sylogs "github.com/calypr/syfon/client/logs"
	syrequest "github.com/calypr/syfon/client/request"
	"github.com/calypr/syfon/client/syfonclient"
)

// SyfonClientInterface groups the syfon service interfaces that data-client needs.
type SyfonClientInterface interface {
	Health() *syfonclient.HealthService
	Data() *syfonclient.DataService
	Index() *syfonclient.IndexService
	DRS() *syfonclient.DRSService
	Buckets() *syfonclient.BucketsService
	Metrics() *syfonclient.MetricsService
	LFS() *syfonclient.LFSService
}

type syfonClient struct {
	health  *syfonclient.HealthService
	data    *syfonclient.DataService
	index   *syfonclient.IndexService
	drs     *syfonclient.DRSService
	buckets *syfonclient.BucketsService
	metrics *syfonclient.MetricsService
	lfs     *syfonclient.LFSService
}

func (c *syfonClient) Health() *syfonclient.HealthService   { return c.health }
func (c *syfonClient) Data() *syfonclient.DataService       { return c.data }
func (c *syfonClient) Index() *syfonclient.IndexService     { return c.index }
func (c *syfonClient) DRS() *syfonclient.DRSService         { return c.drs }
func (c *syfonClient) Buckets() *syfonclient.BucketsService { return c.buckets }
func (c *syfonClient) Metrics() *syfonclient.MetricsService { return c.metrics }
func (c *syfonClient) LFS() *syfonclient.LFSService         { return c.lfs }

type syfonRequestAdapter struct {
	req     request.RequestInterface
	baseURL string
}

func (a *syfonRequestAdapter) Do(ctx context.Context, method, path string, body, out any, opts ...syrequest.RequestOption) error {
	if a.req == nil {
		return fmt.Errorf("request interface is required")
	}

	rb := &syrequest.RequestBuilder{
		Method:  method,
		Url:     a.joinURL(path),
		Headers: map[string]string{},
	}
	for _, opt := range opts {
		opt(rb)
	}

	if rb.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rb.Timeout)
		defer cancel()
	}

	dcRB := a.req.New(method, rb.Url)
	for key, value := range rb.Headers {
		dcRB.WithHeader(key, value)
	}
	if rb.SkipAuth {
		dcRB.WithSkipAuth(true)
	}
	if rb.Token != "" {
		dcRB.WithToken(rb.Token)
	}
	if rb.PartSize != 0 {
		dcRB.WithPartSize(rb.PartSize)
	}

	if body != nil {
		if reader, ok := body.(io.Reader); ok {
			dcRB.WithBody(reader)
		} else {
			var err error
			dcRB, err = dcRB.WithJSONBody(body)
			if err != nil {
				return err
			}
		}
	}

	resp, err := a.req.Do(ctx, dcRB)
	if err != nil {
		return err
	}

	switch target := out.(type) {
	case **http.Response:
		*target = resp
		return nil
	case *http.Response:
		*target = *resp
		return nil
	}

	defer resp.Body.Close()
	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return &syrequest.ResponseError{
			Method:  method,
			URL:     resp.Request.URL.String(),
			Status:  resp.StatusCode,
			Body:    strings.TrimSpace(string(data)),
			Headers: resp.Header.Clone(),
		}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (a *syfonRequestAdapter) joinURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return a.baseURL
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(a.baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func buildSyfonClient(cred *conf.Credential, logger *logs.Gen3Logger, req request.RequestInterface) SyfonClientInterface {
	if cred == nil || req == nil {
		return nil
	}

	baseReq, ok := req.(*request.Request)
	if !ok || baseReq.RetryClient == nil {
		return nil
	}

	httpClient := baseReq.RetryClient.StandardClient()
	var slogLogger *slog.Logger
	if logger != nil {
		slogLogger = logger.Logger
	}
	syLogger := sylogs.NewGen3Logger(slogLogger, "", "")
	syReq := &syfonRequestAdapter{
		req:     req,
		baseURL: cred.APIEndpoint,
	}

	internalClient, err := internalapi.NewClientWithResponses(
		strings.TrimRight(cred.APIEndpoint, "/"),
		internalapi.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil
	}

	drsClient, err := drs.NewClientWithResponses(
		strings.TrimRight(cred.APIEndpoint, "/")+"/ga4gh/drs/v1",
		drs.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil
	}

	bucketClient, err := bucketapi.NewClientWithResponses(
		strings.TrimRight(cred.APIEndpoint, "/"),
		bucketapi.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil
	}

	metricsClient, err := metricsapi.NewClientWithResponses(
		strings.TrimRight(cred.APIEndpoint, "/"),
		metricsapi.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil
	}

	lfsClient, err := lfsapi.NewClientWithResponses(
		strings.TrimRight(cred.APIEndpoint, "/"),
		lfsapi.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil
	}

	index := syfonclient.NewIndexService(internalClient, syReq)
	drsSvc := syfonclient.NewDRSService(drsClient, index)
	dataSvc := syfonclient.NewDataService(internalClient, syReq, syLogger, drsSvc)
	return &syfonClient{
		health:  syfonclient.NewHealthService(syReq),
		data:    dataSvc,
		index:   index,
		drs:     drsSvc,
		buckets: syfonclient.NewBucketsService(bucketClient),
		metrics: syfonclient.NewMetricsService(metricsClient),
		lfs:     syfonclient.NewLFSService(lfsClient),
	}
}
