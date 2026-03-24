package fence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"log/slog"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/request"
	"github.com/hashicorp/go-version"
)

// FenceBucketEndpoint is the endpoint postfix for FENCE bucket list
const FenceBucketEndpoint = "/data/buckets"

//go:generate mockgen -destination=../mocks/mock_fence.go -package=mocks github.com/calypr/data-client/fence FenceInterface

// FenceInterface defines the interface for Fence client
type FenceInterface interface {
	request.RequestInterface

	NewAccessToken(ctx context.Context) (string, error)
	CheckPrivileges(ctx context.Context) (map[string]any, error)
	CheckForShepherdAPI(ctx context.Context) (bool, error)
	DeleteRecord(ctx context.Context, guid string) (string, error)
	GetDownloadPresignedUrl(ctx context.Context, guid, protocolText string) (string, error)

	UserPing(ctx context.Context) (*PingResp, error)

	// Bucket details
	GetBucketDetails(ctx context.Context, bucket string) (*S3Bucket, error)

	// Upload methods
	InitUpload(ctx context.Context, filename string, bucket string, guid string) (FenceResponse, error)
	GetUploadPresignedUrl(ctx context.Context, guid string, filename string, bucket string) (FenceResponse, error)

	// Multipart methods
	InitMultipartUpload(ctx context.Context, filename string, bucket string, guid string) (FenceResponse, error)
	GenerateMultipartPresignedURL(ctx context.Context, key string, uploadID string, partNumber int, bucket string) (string, error)
	CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []MultipartPart, bucket string) error
	ParseFenceURLResponse(resp *http.Response) (FenceResponse, error)

	RefreshToken(ctx context.Context) error
}

// FenceClient implements FenceInterface
// FenceClient implements FenceInterface
type FenceClient struct {
	request.RequestInterface
	cred   *conf.Credential
	logger *slog.Logger
}

// NewFenceClient creates a new FenceClient
func NewFenceClient(req request.RequestInterface, cred *conf.Credential, logger *slog.Logger) FenceInterface {
	return &FenceClient{
		RequestInterface: req,
		cred:             cred,
		logger:           logger,
	}
}

func (f *FenceClient) NewAccessToken(ctx context.Context) (string, error) {
	if f.cred.APIKey == "" {
		return "", errors.New("APIKey is required to refresh access token")
	}

	payload, err := json.Marshal(map[string]string{"api_key": f.cred.APIKey})
	if err != nil {
		return "", err
	}
	bodyReader := bytes.NewReader(payload)

	resp, err := f.Do(
		ctx,
		&request.RequestBuilder{
			Method:  http.MethodPost,
			Url:     f.cred.APIEndpoint + common.FenceAccessTokenEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Body:    bodyReader,
		},
	)

	if err != nil {
		return "", fmt.Errorf("error when calling Request.Do: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("failed to refresh token, status: " + strconv.Itoa(resp.StatusCode))
	}

	var result common.AccessTokenStruct
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errors.New("failed to parse token response: " + err.Error())
	}

	return result.AccessToken, nil
}

func (f *FenceClient) RefreshToken(ctx context.Context) error {
	token, err := f.NewAccessToken(ctx)
	if err != nil {
		return err
	}
	f.cred.AccessToken = token
	return nil
}

func (f *FenceClient) CheckPrivileges(ctx context.Context) (map[string]any, error) {
	resp, err := f.Do(ctx,
		&request.RequestBuilder{
			Url:    f.cred.APIEndpoint + common.FenceUserEndpoint,
			Method: http.MethodGet,
			Token:  f.cred.AccessToken,
		},
	)
	if err != nil {
		return nil, errors.New("error occurred when getting response from remote: " + err.Error())
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data map[string]any
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		return nil, errors.New("error occurred when unmarshalling response: " + err.Error())
	}

	resourceAccess, ok := data["authz"].(map[string]any)

	// If the `authz` section (Arborist permissions) is empty or missing, try get `project_access` section (Fence permissions)
	if len(resourceAccess) == 0 || !ok {
		resourceAccess, ok = data["project_access"].(map[string]any)
		if !ok {
			return nil, errors.New("not possible to read access privileges of user")
		}
	}

	return resourceAccess, nil
}

func (f *FenceClient) CheckForShepherdAPI(ctx context.Context) (bool, error) {
	// Check if Shepherd is enabled
	if f.cred.UseShepherd == "false" {
		return false, nil
	}
	if f.cred.UseShepherd != "true" && common.DefaultUseShepherd == false {
		return false, nil
	}
	// If Shepherd is enabled, make sure that the commons has a compatible version of Shepherd deployed.
	// Compare the version returned from the Shepherd version endpoint with the minimum acceptable Shepherd version.
	var minShepherdVersion string
	if f.cred.MinShepherdVersion == "" {
		minShepherdVersion = common.DefaultMinShepherdVersion
	} else {
		minShepherdVersion = f.cred.MinShepherdVersion
	}

	res, err := f.Do(ctx,
		&request.RequestBuilder{
			Url:    f.cred.APIEndpoint + common.ShepherdVersionEndpoint,
			Method: http.MethodGet,
			Token:  f.cred.AccessToken,
		},
	)
	if err != nil {
		return false, errors.New("Error occurred during generating HTTP request: " + err.Error())
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return false, nil
	}
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return false, errors.New("Error occurred when reading HTTP request: " + err.Error())
	}
	body, err := strconv.Unquote(string(bodyBytes))
	if err != nil {
		return false, fmt.Errorf("Error occurred when parsing version from Shepherd: %v: %v", string(body), err)
	}
	// Compare the version in the response to the target version
	ver, err := version.NewVersion(body)
	if err != nil {
		return false, fmt.Errorf("Error occurred when parsing version from Shepherd: %v: %v", string(body), err)
	}
	minVer, err := version.NewVersion(minShepherdVersion)
	if err != nil {
		return false, fmt.Errorf("Error occurred when parsing minimum acceptable Shepherd version: %v: %v", minShepherdVersion, err)
	}
	if ver.GreaterThanOrEqual(minVer) {
		return true, nil
	}
	return false, fmt.Errorf("Shepherd is enabled, but %v does not have correct Shepherd version. (Need Shepherd version >=%v, got %v)", f.cred.APIEndpoint, minVer, ver)
}

func (f *FenceClient) DeleteRecord(ctx context.Context, guid string) (string, error) {
	hasShepherd, err := f.CheckForShepherdAPI(ctx)
	if err != nil {
		f.logger.Warn(fmt.Sprintf("WARNING: Error checking Shepherd API: %v. Falling back to Fence.\n", err))
	} else if hasShepherd {
		resp, err := f.Do(ctx,
			&request.RequestBuilder{
				Url:    f.cred.APIEndpoint + common.ShepherdEndpoint + "/objects/" + guid,
				Method: http.MethodDelete,
				Token:  f.cred.AccessToken,
			},
		)
		if err != nil {
			return "", fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 204 {
			return "Record with GUID " + guid + " has been deleted", nil
		}
		return "", fmt.Errorf("shepherd delete failed: %d", resp.StatusCode)
	}

	resp, err := f.Do(ctx,
		&request.RequestBuilder{
			Url:    f.cred.APIEndpoint + common.FenceDataEndpoint + "/" + guid,
			Method: http.MethodDelete,
			Token:  f.cred.AccessToken,
		},
	)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return "Record with GUID " + guid + " has been deleted", nil
	}

	_, err = f.ParseFenceURLResponse(resp)
	if err != nil {
		return "", err
	}
	return "Record with GUID " + guid + " has been deleted", nil
}

func (f *FenceClient) GetDownloadPresignedUrl(ctx context.Context, guid, protocolText string) (string, error) {
	hasShepherd, err := f.CheckForShepherdAPI(ctx)
	if err == nil && hasShepherd {
		return f.resolveFromShepherd(ctx, guid)
	}
	return f.resolveFromFence(ctx, guid, protocolText)
}

func (f *FenceClient) resolveFromShepherd(ctx context.Context, guid string) (string, error) {
	url := fmt.Sprintf("%s%s/objects/%s/download", f.cred.APIEndpoint, common.ShepherdEndpoint, guid)
	resp, err := f.Do(ctx, &request.RequestBuilder{
		Url:    url,
		Method: http.MethodGet,
		Token:  f.cred.AccessToken,
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("shepherd error: %d", resp.StatusCode)
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode shepherd response: %w", err)
	}

	return result.URL, nil
}

func (f *FenceClient) resolveFromFence(ctx context.Context, guid, protocolText string) (string, error) {
	resp, err := f.Do(
		ctx,
		&request.RequestBuilder{
			Url:    f.cred.APIEndpoint + common.FenceDataDownloadEndpoint + "/" + guid + protocolText,
			Method: http.MethodGet,
			Token:  f.cred.AccessToken,
		},
	)
	if err != nil {
		return "", errors.New("failed to get URL from Fence via Do: " + err.Error())
	}
	defer resp.Body.Close()

	msg, err := f.ParseFenceURLResponse(resp)
	if err != nil || msg.URL == "" {
		return "", errors.New("failed to get URL from Fence via ParseFenceURLResponse: " + err.Error())
	}

	return msg.URL, nil
}

func (f *FenceClient) GetBucketDetails(ctx context.Context, bucket string) (*S3Bucket, error) {
	url := f.cred.APIEndpoint + "/data/buckets"
	resp, err := f.Do(ctx, &request.RequestBuilder{
		Method: http.MethodGet,
		Url:    url,
		Token:  f.cred.AccessToken,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bucket information: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var bucketInfo S3BucketsResponse
	if err := json.NewDecoder(resp.Body).Decode(&bucketInfo); err != nil {
		return nil, fmt.Errorf("failed to decode bucket information: %w", err)
	}

	if info, exists := bucketInfo.S3Buckets[bucket]; exists {
		if info.EndpointURL != "" && info.Region != "" {
			return info, nil
		}
		return nil, errors.New("endpoint_url or region not found for bucket")
	}

	return nil, nil
}

func (f *FenceClient) InitUpload(ctx context.Context, filename string, bucket string, guid string) (FenceResponse, error) {
	payload := map[string]string{
		"file_name": filename,
	}
	if bucket != "" {
		payload["bucket"] = bucket
	}
	if guid != "" {
		payload["guid"] = guid
	}

	buf, err := common.ToJSONReader(payload)
	if err != nil {
		return FenceResponse{}, err
	}

	resp, err := f.Do(
		ctx,
		&request.RequestBuilder{
			Method:  http.MethodPost,
			Url:     f.cred.APIEndpoint + common.FenceDataUploadEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Body:    buf,
			Token:   f.cred.AccessToken,
		})
	if err != nil {
		return FenceResponse{}, err
	}
	defer resp.Body.Close()

	return f.ParseFenceURLResponse(resp)
}

func (f *FenceClient) GetUploadPresignedUrl(ctx context.Context, guid string, filename string, bucket string) (FenceResponse, error) {
	endPointPostfix := common.FenceDataUploadEndpoint + "/" + guid + "?file_name=" + url.QueryEscape(filename)
	if bucket != "" {
		endPointPostfix += "&bucket=" + bucket
	}

	resp, err := f.Do(
		ctx,
		&request.RequestBuilder{
			Url:     f.cred.APIEndpoint + endPointPostfix,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Token:   f.cred.AccessToken,
			Method:  http.MethodGet,
		},
	)
	if err != nil {
		return FenceResponse{}, err
	}
	defer resp.Body.Close()

	return f.ParseFenceURLResponse(resp)
}

func (f *FenceClient) InitMultipartUpload(ctx context.Context, filename string, bucket string, guid string) (FenceResponse, error) {
	reader, err := common.ToJSONReader(
		InitRequestObject{
			Filename: filename,
			Bucket:   bucket,
			GUID:     guid,
		},
	)
	if err != nil {
		return FenceResponse{}, err
	}

	resp, err := f.Do(
		ctx,
		&request.RequestBuilder{
			Method:  http.MethodPost,
			Url:     f.cred.APIEndpoint + common.FenceDataMultipartInitEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Body:    reader,
			Token:   f.cred.AccessToken,
		},
	)

	if err != nil {
		return FenceResponse{}, err
	}
	defer resp.Body.Close()

	return f.ParseFenceURLResponse(resp)
}

func (f *FenceClient) GenerateMultipartPresignedURL(ctx context.Context, key string, uploadID string, partNumber int, bucket string) (string, error) {
	reader, err := common.ToJSONReader(
		MultipartUploadRequestObject{
			Key:        key,
			UploadID:   uploadID,
			PartNumber: partNumber,
			Bucket:     bucket,
		},
	)
	if err != nil {
		return "", err
	}

	resp, err := f.Do(
		ctx,
		&request.RequestBuilder{
			Url:     f.cred.APIEndpoint + common.FenceDataMultipartUploadEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Method:  http.MethodPost,
			Body:    reader,
			Token:   f.cred.AccessToken,
		},
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	msg, err := f.ParseFenceURLResponse(resp)
	if err != nil {
		return "", err
	}

	return msg.PresignedURL, nil
}

func (f *FenceClient) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []MultipartPart, bucket string) error {
	multipartCompleteObject := MultipartCompleteRequestObject{Key: key, UploadID: uploadID, Parts: parts, Bucket: bucket}

	reader, err := common.ToJSONReader(multipartCompleteObject)
	if err != nil {
		return err
	}

	resp, err := f.Do(
		ctx,
		&request.RequestBuilder{
			Url:     f.cred.APIEndpoint + common.FenceDataMultipartCompleteEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Body:    reader,
			Method:  http.MethodPost,
			Token:   f.cred.AccessToken,
		},
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	_, err = f.ParseFenceURLResponse(resp)
	return err
}

func (f *FenceClient) ParseFenceURLResponse(resp *http.Response) (FenceResponse, error) {
	msg := FenceResponse{}
	if resp == nil {
		return msg, errors.New("nil response received")
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return msg, fmt.Errorf("failed to read response body: %w", err)
	}
	bodyStr := string(bodyBytes)
	strURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		strURL = resp.Request.URL.String()
	}

	// Handle HTTP error statuses first so plain-text error bodies (for example:
	// "Unauthorized") are reported accurately instead of as JSON decode failures.
	if !(resp.StatusCode == 200 || resp.StatusCode == 201 || resp.StatusCode == 204) {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return msg, fmt.Errorf("401 Unauthorized: %s (URL: %s)", bodyStr, strURL)
		case http.StatusForbidden:
			return msg, fmt.Errorf("403 Forbidden: %s (URL: %s)", bodyStr, strURL)
		case http.StatusNotFound:
			return msg, fmt.Errorf("404 Not Found: %s (URL: %s)", bodyStr, strURL)
		case http.StatusInternalServerError:
			return msg, fmt.Errorf("500 Internal Server Error: %s (URL: %s)", bodyStr, strURL)
		case http.StatusServiceUnavailable:
			return msg, fmt.Errorf("503 Service Unavailable: %s (URL: %s)", bodyStr, strURL)
		case http.StatusBadGateway:
			return msg, fmt.Errorf("502 Bad Gateway: %s (URL: %s)", bodyStr, strURL)
		default:
			return msg, fmt.Errorf("unexpected error (%d): %s (URL: %s)", resp.StatusCode, bodyStr, strURL)
		}
	}

	if len(bodyBytes) > 0 {
		err = json.Unmarshal(bodyBytes, &msg)
		if err != nil {
			return msg, fmt.Errorf("failed to decode JSON response (status=%d, url=%s): %w (raw body: %s)", resp.StatusCode, strURL, err, bodyStr)
		}
	}

	if strings.Contains(bodyStr, "Can't find a location for the data") {
		return msg, errors.New("the provided GUID is not found")
	}

	return msg, nil
}

func (f *FenceClient) UserPing(ctx context.Context) (*PingResp, error) {
	resp, err := f.Do(ctx, &request.RequestBuilder{
		Url:    f.cred.APIEndpoint + common.FenceUserEndpoint,
		Method: http.MethodGet,
		Token:  f.cred.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info, status: %d", resp.StatusCode)
	}

	var uResp FenceUserResp
	if err := json.NewDecoder(resp.Body).Decode(&uResp); err != nil {
		return nil, err
	}

	bucketResp, err := f.Do(ctx, &request.RequestBuilder{
		Url:    f.cred.APIEndpoint + FenceBucketEndpoint,
		Method: http.MethodGet,
		Token:  f.cred.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	defer bucketResp.Body.Close()

	if bucketResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get bucket info, status: %d", bucketResp.StatusCode)
	}

	var bResp S3BucketsResponse
	if err := json.NewDecoder(bucketResp.Body).Decode(&bResp); err != nil {
		return nil, err
	}

	return &PingResp{
		Profile:        f.cred.Profile,
		Username:       uResp.Username,
		Endpoint:       f.cred.APIEndpoint,
		BucketPrograms: ParseBucketResp(bResp),
		YourAccess:     ParseUserResp(uResp),
	}, nil
}

func ParseBucketResp(resp S3BucketsResponse) map[string]string {
	bucketsByProgram := make(map[string]string)

	// Check both S3_BUCKETS and s3_buckets
	s3Buckets := resp.S3Buckets
	if len(s3Buckets) == 0 {
		s3Buckets = resp.S3BucketsLower
	}

	for bucketName, bucketInfo := range s3Buckets {
		var programs strings.Builder
		if len(bucketInfo.Programs) > 1 {
			for i, p := range bucketInfo.Programs {
				if i > 0 {
					programs.WriteString(",")
				}
				programs.WriteString(p)
			}
		} else if len(bucketInfo.Programs) == 1 {
			programs.WriteString(bucketInfo.Programs[0])
		}
		bucketsByProgram[bucketName] = programs.String()
	}
	return bucketsByProgram
}

func ParseUserResp(resp FenceUserResp) map[string]string {
	servicesByPath := make(map[string]string)
	for path, permissions := range resp.Authz {
		var services strings.Builder
		seenServices := make(map[string]bool)
		for _, p := range permissions {
			if !seenServices[p.Method] {
				if services.Len() > 0 {
					services.WriteString(",")
				}
				services.WriteString(p.Method)
				seenServices[p.Method] = true
			}
		}
		if services.Len() > 0 {
			servicesByPath[path] = services.String()
		}
	}
	return servicesByPath
}
