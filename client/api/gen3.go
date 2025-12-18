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
	"net/url"
	"strconv"
	"strings"

	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/conf"
	req "github.com/calypr/data-client/client/request"
	"github.com/hashicorp/go-version"
)

func NewFunctions(ctx context.Context, config conf.Manager, request req.RequestInterface) FunctionInterface {
	return &Functions{
		Request: request,
	}
}

type Functions struct {
	Config  conf.ManagerInterface
	Request req.RequestInterface
}

type FunctionInterface interface {
	CheckPrivileges(cred *conf.Credential) (string, map[string]any, error)
	CheckForShepherdAPI(cred *conf.Credential) (bool, error)
	GetResponse(cred *conf.Credential, endpointPostPrefix string, method string, contentType string, bodyBytes []byte) (string, *http.Response, error)
	DoRequestWithSignedHeader(cred *conf.Credential, endpointPostPrefix string, contentType string, bodyBytes []byte) (FenceResponse, error)
	GetHost(cred *conf.Credential) (*url.URL, error)
	ExportCredential(cred *conf.Credential) error

	ParseFenceURLResponse(resp *http.Response) (FenceResponse, error)
}

func (f *Functions) ParseFenceURLResponse(resp *http.Response) (FenceResponse, error) {
	msg := FenceResponse{}

	if resp == nil {
		return msg, errors.New("Nil response received")
	}

	// Capture the body for error reporting before we do anything else
	// Using your existing ResponseToString helper
	bodyStr := ResponseToString(resp)

	if !(resp.StatusCode == 200 || resp.StatusCode == 201) {
		// Prepare a base error that includes the body content
		errorMessage := fmt.Sprintf("Status: %d | Response: %s", resp.StatusCode, bodyStr)

		switch resp.StatusCode {
		case 401:
			return msg, fmt.Errorf("401 Unauthorized: %s", errorMessage)
		case 403:
			return msg, fmt.Errorf("403 Forbidden: %s (URL: %s)", bodyStr, resp.Request.URL.String())
		case 404:
			return msg, fmt.Errorf("404 Not Found: %s (URL: %s)", bodyStr, resp.Request.URL.String())
		case 500:
			return msg, fmt.Errorf("500 Internal Server Error: %s", bodyStr)
		case 503:
			return msg, fmt.Errorf("503 Service Unavailable: %s", bodyStr)
		default:
			return msg, fmt.Errorf("Unexpected Error (%d): %s", resp.StatusCode, bodyStr)
		}
	}

	// Logic for successful status codes
	if strings.Contains(bodyStr, "Can't find a location for the data") {
		return msg, errors.New("The provided GUID is not found")
	}

	err := json.Unmarshal([]byte(bodyStr), &msg)
	if err != nil {
		return msg, fmt.Errorf("failed to decode JSON: %w (Raw body: %s)", err, bodyStr)
	}

	return msg, nil
}
func (f *Functions) CheckForShepherdAPI(profileConfig *conf.Credential) (bool, error) {
	// Check if Shepherd is enabled
	if profileConfig.UseShepherd == "false" {
		return false, nil
	}
	if profileConfig.UseShepherd != "true" && common.DefaultUseShepherd == false {
		return false, nil
	}
	// If Shepherd is enabled, make sure that the commons has a compatible version of Shepherd deployed.
	// Compare the version returned from the Shepherd version endpoint with the minimum acceptable Shepherd version.
	var minShepherdVersion string
	if profileConfig.MinShepherdVersion == "" {
		minShepherdVersion = common.DefaultMinShepherdVersion
	} else {
		minShepherdVersion = profileConfig.MinShepherdVersion
	}

	_, res, err := f.GetResponse(profileConfig, common.ShepherdVersionEndpoint, "GET", "", nil)
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
	return false, fmt.Errorf("Shepherd is enabled, but %v does not have correct Shepherd version. (Need Shepherd version >=%v, got %v)", profileConfig.APIEndpoint, minVer, ver)
}

func (f *Functions) GetResponse(profileConfig *conf.Credential, endpointPostPrefix string, method string, contentType string, bodyBytes []byte) (string, *http.Response, error) {

	var resp *http.Response
	var err error

	if profileConfig.APIKey == "" && profileConfig.AccessToken == "" && profileConfig.APIEndpoint == "" {
		return "", resp, fmt.Errorf("No credentials found in the configuration file! Please use \"./data-client configure\" to configure your credentials first %s", profileConfig)
	}

	host, _ := url.Parse(profileConfig.APIEndpoint)
	prefixEndPoint := host.Scheme + "://" + host.Host
	apiEndpoint := host.Scheme + "://" + host.Host + endpointPostPrefix
	isExpiredToken := false
	if profileConfig.AccessToken != "" {
		resp, err = f.Request.MakeARequest(method, apiEndpoint, profileConfig.AccessToken, contentType, nil, bytes.NewBuffer(bodyBytes), false)
		if err != nil {
			return "", resp, fmt.Errorf("Error while requesting user access token at %v: %v", apiEndpoint, err)
		}

		// 401 code is general error code from FENCE. the error message is also not clear for the case
		// that the token expired. Temporary solution: get new access token and make another attempt.
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 503) {
			isExpiredToken = true
		} else {
			return prefixEndPoint, resp, err
		}
	}
	if profileConfig.AccessToken == "" || isExpiredToken {
		err := f.Request.RequestNewAccessToken(prefixEndPoint+common.FenceAccessTokenEndpoint, profileConfig)
		if err != nil {
			return prefixEndPoint, resp, err
		}
		err = f.Config.Save(profileConfig)
		if err != nil {
			return prefixEndPoint, resp, err
		}

		resp, err = f.Request.MakeARequest(method, apiEndpoint, profileConfig.AccessToken, contentType, nil, bytes.NewBuffer(bodyBytes), false)
		if err != nil {
			return prefixEndPoint, resp, err
		}
	}

	return prefixEndPoint, resp, nil
}

func (f *Functions) GetHost(profileConfig *conf.Credential) (*url.URL, error) {
	if profileConfig.APIEndpoint == "" {
		return nil, errors.New("No APIEndpoint found in the configuration file! Please use \"./data-client configure\" to configure your credentials first")
	}
	host, _ := url.Parse(profileConfig.APIEndpoint)
	return host, nil
}

func (f *Functions) DoRequestWithSignedHeader(profileConfig *conf.Credential, endpointPostPrefix string, contentType string, bodyBytes []byte) (FenceResponse, error) {
	/*
	   Do request with signed header. User may have more than one profile and use a profile to make a request
	*/
	var err error
	var msg FenceResponse

	method := "GET"
	if bodyBytes != nil {
		method = "POST"
	}

	_, resp, err := f.GetResponse(profileConfig, endpointPostPrefix, method, contentType, bodyBytes)
	if err != nil {
		return msg, err
	}
	defer resp.Body.Close()

	msg, err = f.ParseFenceURLResponse(resp)
	return msg, err
}

func (f *Functions) CheckPrivileges(profileConfig *conf.Credential) (string, map[string]any, error) {
	/*
	   Return user privileges from specified profile
	*/
	var err error
	var data map[string]any

	host, resp, err := f.GetResponse(profileConfig, common.FenceUserEndpoint, "GET", "", nil)
	if err != nil {
		return "", nil, errors.New("Error occurred when getting response from remote: " + err.Error())
	}
	defer resp.Body.Close()

	str := ResponseToString(resp)
	err = json.Unmarshal([]byte(str), &data)
	if err != nil {
		return "", nil, errors.New("Error occurred when unmarshalling response: " + err.Error())
	}

	resourceAccess, ok := data["authz"].(map[string]any)

	// If the `authz` section (Arborist permissions) is empty or missing, try get `project_access` section (Fence permissions)
	if len(resourceAccess) == 0 || !ok {
		resourceAccess, ok = data["project_access"].(map[string]any)
		if !ok {
			return "", nil, errors.New("Not possible to read access privileges of user")
		}
	}

	return host, resourceAccess, err
}

func (f *Functions) DeleteRecord(profileConfig *conf.Credential, guid string) (string, error) {
	endpoint := common.FenceDataEndpoint + "/" + guid
	hasShepherd, err := f.CheckForShepherdAPI(profileConfig)
	if err != nil {
		f.Request.Logger().Printf("WARNING: Error checking Shepherd API: %v. Falling back to Fence.\n", err)
	} else if hasShepherd {
		endpoint = common.ShepherdEndpoint + "/objects/" + guid
	}

	_, resp, err := f.GetResponse(profileConfig, endpoint, "DELETE", "", nil)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyMsg := string(bodyBytes)

	switch resp.StatusCode {
	case 204:
		return fmt.Sprintf("Record with GUID %s has been deleted. Response: %s", guid, bodyMsg), nil
	case 500:
		return bodyMsg, fmt.Errorf("internal server error (500) for GUID %s: could not delete stored files or INDEXD record. Response: %s", guid, bodyMsg)
	default:
		return bodyMsg, fmt.Errorf("unexpected error (%d) for GUID %s. Response: %s", resp.StatusCode, guid, bodyMsg)
	}
}

func (f *Functions) ExportCredential(cred *conf.Credential) error {
	var request req.Request = req.Request{Ctx: context.Background()}

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
		err = request.RequestNewAccessToken(fenceBase+common.FenceAccessTokenEndpoint, cred)
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
		f.Request.Logger().Printf("WARNING: Your profile will only be valid for 24 hours since you have only provided a refresh token for authentication")
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
