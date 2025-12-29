package api

//go:generate mockgen -destination=../mocks/mock_functions.go -package=mocks github.com/calypr/data-client/client/api FunctionInterface

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/logs"
	"github.com/calypr/data-client/client/request"
	"github.com/hashicorp/go-version"
)

func NewFunctions(config conf.ManagerInterface, request request.RequestInterface, cred *conf.Credential, logger logs.Logger) FunctionInterface {
	return &Functions{
		RequestInterface: request,
		Cred:             cred,
		Config:           config,
		Logger:           logger,
	}
}

type Functions struct {
	request.RequestInterface

	Cred   *conf.Credential
	Config conf.ManagerInterface
	Logger logs.Logger
}

type FunctionInterface interface {
	request.RequestInterface

	CheckPrivileges(ctx context.Context) (map[string]any, error)
	CheckForShepherdAPI(ctx context.Context) (bool, error)
	DeleteRecord(ctx context.Context, guid string) (string, error)
	GetDownloadPresignedUrl(ctx context.Context, guid, protocolText string) (string, error)

	ParseFenceURLResponse(resp *http.Response) (FenceResponse, error)
	ExportCredential(ctx context.Context, cred *conf.Credential) error
	NewAccessToken(ctx context.Context) error
}

func (f *Functions) NewAccessToken(ctx context.Context) error {
	if f.Cred.APIKey == "" {
		return errors.New("APIKey is required to refresh access token")
	}

	payload, err := json.Marshal(map[string]string{"api_key": f.Cred.APIKey})
	if err != nil {
		return err
	}
	bodyReader := bytes.NewReader(payload)

	resp, err := f.Do(
		ctx,
		f.New(http.MethodPost, f.Cred.APIEndpoint+common.FenceAccessTokenEndpoint).
			WithHeader(common.HeaderContentType, common.MIMEApplicationJSON).
			WithBody(bodyReader),
	)

	if err != nil {
		return fmt.Errorf("Error when calling Request.Do: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("failed to refresh token, status: " + strconv.Itoa(resp.StatusCode))
	}

	var result common.AccessTokenStruct
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return errors.New("failed to parse token response: " + err.Error())
	}

	f.Cred.AccessToken = result.AccessToken
	return nil
}

func (f *Functions) GetDownloadPresignedUrl(ctx context.Context, guid, protocolText string) (string, error) {
	hasShepherd, err := f.CheckForShepherdAPI(ctx) // error already logged upstream
	if err == nil && hasShepherd {
		return f.resolveFromShepherd(ctx, guid)
	}
	return f.resolveFromFence(ctx, guid, protocolText)
}

// Todo: why isn't this calld in every fence response that has a body ? why is this seperated out
func (f *Functions) ParseFenceURLResponse(resp *http.Response) (FenceResponse, error) {
	msg := FenceResponse{}
	if resp == nil {
		return msg, errors.New("Nil response received")
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	bodyStr := buf.String()

	err := json.Unmarshal(buf.Bytes(), &msg)
	if err != nil {
		return msg, fmt.Errorf("failed to decode JSON: %w (Raw body: %s)", err, buf.String())
	}

	if !(resp.StatusCode == 200 || resp.StatusCode == 201 || resp.StatusCode == 204) {
		strUrl := resp.Request.URL.String()
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return msg, fmt.Errorf("401 Unauthorized: %s (URL: %s)", bodyStr, strUrl)
		case http.StatusForbidden:
			return msg, fmt.Errorf("403 Forbidden: %s (URL: %s)", bodyStr, strUrl)
		case http.StatusNotFound:
			return msg, fmt.Errorf("404 Not Found: %s (URL: %s)", bodyStr, strUrl)
		case http.StatusInternalServerError:
			return msg, fmt.Errorf("500 Internal Server Error: %s (URL: %s)", bodyStr, strUrl)
		case http.StatusServiceUnavailable:
			return msg, fmt.Errorf("503 Service Unavailable: %s (URL: %s)", bodyStr, strUrl)
		case http.StatusBadGateway:
			return msg, fmt.Errorf("502 Bad Gateway: %s (URL: %s)", bodyStr, strUrl)
		default:
			return msg, fmt.Errorf("Unexpected Error (%d): %s (URL: %s)", resp.StatusCode, bodyStr, strUrl)
		}
	}

	// Logic for successful status codes
	if strings.Contains(bodyStr, "Can't find a location for the data") {
		return msg, errors.New("The provided GUID is not found")
	}

	return msg, nil
}

func (f *Functions) CheckForShepherdAPI(ctx context.Context) (bool, error) {
	// Check if Shepherd is enabled
	if f.Cred.UseShepherd == "false" {
		return false, nil
	}
	if f.Cred.UseShepherd != "true" && common.DefaultUseShepherd == false {
		return false, nil
	}
	// If Shepherd is enabled, make sure that the commons has a compatible version of Shepherd deployed.
	// Compare the version returned from the Shepherd version endpoint with the minimum acceptable Shepherd version.
	var minShepherdVersion string
	if f.Cred.MinShepherdVersion == "" {
		minShepherdVersion = common.DefaultMinShepherdVersion
	} else {
		minShepherdVersion = f.Cred.MinShepherdVersion
	}

	res, err := f.Do(ctx,
		&request.RequestBuilder{
			Url:    f.Cred.APIEndpoint + common.ShepherdVersionEndpoint,
			Method: http.MethodGet,
			Token:  f.Cred.AccessToken,
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
	return false, fmt.Errorf("Shepherd is enabled, but %v does not have correct Shepherd version. (Need Shepherd version >=%v, got %v)", f.Cred.APIEndpoint, minVer, ver)
}
func (f *Functions) CheckPrivileges(ctx context.Context) (map[string]any, error) {
	/*
	   Return user privileges from specified profile
	*/
	var err error
	var data map[string]any

	resp, err := f.Do(ctx,
		&request.RequestBuilder{
			Url:    f.Cred.APIEndpoint + common.FenceUserEndpoint,
			Method: http.MethodGet,
			Token:  f.Cred.AccessToken,
		},
	)
	if err != nil {
		return nil, errors.New("Error occurred when getting response from remote: " + err.Error())
	}
	defer resp.Body.Close()

	str := ResponseToString(resp)
	err = json.Unmarshal([]byte(str), &data)
	if err != nil {
		return nil, errors.New("Error occurred when unmarshalling response: " + err.Error())
	}

	resourceAccess, ok := data["authz"].(map[string]any)

	// If the `authz` section (Arborist permissions) is empty or missing, try get `project_access` section (Fence permissions)
	if len(resourceAccess) == 0 || !ok {
		resourceAccess, ok = data["project_access"].(map[string]any)
		if !ok {
			return nil, errors.New("Not possible to read access privileges of user")
		}
	}

	return resourceAccess, err
}

func (f *Functions) DeleteRecord(ctx context.Context, guid string) (string, error) {
	endpoint := common.FenceDataEndpoint + "/" + guid
	msg := ""
	hasShepherd, err := f.CheckForShepherdAPI(ctx)
	if err != nil {
		f.Logger.Printf("WARNING: Error checking Shepherd API: %v. Falling back to Fence.\n", err)
	} else if hasShepherd {
		endpoint = common.ShepherdEndpoint + "/objects/" + guid
	}

	resp, err := f.Do(ctx,
		&request.RequestBuilder{
			Url:    f.Cred.APIEndpoint + endpoint,
			Method: http.MethodDelete,
			Token:  f.Cred.AccessToken,
		},
	)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		msg = "Record with GUID " + guid + " has been deleted"
	} else {
		_, err = f.ParseFenceURLResponse(resp)
		if err != nil {
			return "", err
		}
	}
	return msg, nil
}

func (f *Functions) ExportCredential(ctx context.Context, cred *conf.Credential) error {

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
	if _, err := f.Config.Load(cred.Profile); err != nil && !errors.Is(err, conf.ErrProfileNotFound) {
		return err
	}

	if cred.APIKey != "" {
		// Always refresh the access token — ignore any old one that might be in the struct
		err = f.NewAccessToken(ctx)
		if err != nil {
			if strings.Contains(err.Error(), "401") {
				return fmt.Errorf("authentication failed (401) for %s — your API key is invalid, revoked, or expired", fenceBase)
			}
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "no such host") {
				return fmt.Errorf("cannot reach Fence at %s — is this a valid Gen3 commons?", fenceBase)
			}
			return fmt.Errorf("failed to refresh access token: %w", err)
		}
	} else {
		f.Logger.Printf("WARNING: Your profile will only be valid for 24 hours since you have only provided a refresh token for authentication")
	}

	// Clean up shepherd flags
	cred.UseShepherd = strings.TrimSpace(cred.UseShepherd)
	cred.MinShepherdVersion = strings.TrimSpace(cred.MinShepherdVersion)

	if cred.MinShepherdVersion != "" {
		if _, err = version.NewVersion(cred.MinShepherdVersion); err != nil {
			return fmt.Errorf("invalid min-shepherd-version: %w", err)
		}
	}

	if err := f.Config.Save(cred); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (f *Functions) resolveFromShepherd(ctx context.Context, guid string) (string, error) {
	// We use f.Cred.APIEndpoint because the struct owns the credential state
	url := fmt.Sprintf("%s%s/objects/%s/download", f.Cred.APIEndpoint, common.ShepherdEndpoint, guid)

	// We call f.Do directly because of method promotion (embedding)
	resp, err := f.Do(ctx, f.New(http.MethodGet, url))
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

func (f *Functions) resolveFromFence(ctx context.Context, guid, protocolText string) (string, error) {
	resp, err := f.Do(
		ctx,
		&request.RequestBuilder{
			Url:    f.Cred.APIEndpoint + common.FenceDataDownloadEndpoint + "/" + guid + protocolText,
			Method: http.MethodGet,
			Token:  f.Cred.AccessToken,
		},
	)
	if err != nil {
		return "", errors.New("Failed to get URL from Fence via DoAuthenticatedRequest: " + err.Error())
	}
	defer resp.Body.Close()

	msg, err := f.ParseFenceURLResponse(resp)
	if err != nil || msg.URL == "" {
		return "", errors.New("Failed to get URL from Fence via ParseFenceURLResponse: " + err.Error())
	}

	return msg.URL, nil
}
