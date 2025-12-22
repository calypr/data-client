package api

//go:generate mockgen -destination=../mocks/mock_functions.go -package=mocks github.com/calypr/data-client/client/api FunctionInterface

import (
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
	"github.com/calypr/data-client/client/logs"
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
	Logger  logs.Logger
}

type FunctionInterface interface {
	ParseFenceURLResponse(resp *http.Response) (FenceResponse, error)

	CheckPrivileges(cred *conf.Credential) (map[string]any, error)
	CheckForShepherdAPI(cred *conf.Credential) (bool, error)
	ExportCredential(cred *conf.Credential) error

	DoAuthenticatedRequest(cred *conf.Credential, request *req.RequestBuilder) (*http.Response, error)
	DeleteRecord(profileConfig *conf.Credential, guid string) (string, error)
}

func (r *Functions) NewAccessToken(profileConfig *conf.Credential) error {
	if profileConfig.APIKey == "" {
		return errors.New("APIKey is required to refresh access token")
	}

	bodyBytes, err := json.Marshal(map[string]string{"api_key": profileConfig.APIKey})
	if err != nil {
		return err
	}

	resp, err := r.Request.Do(
		r.Request.New(http.MethodPost, profileConfig.APIEndpoint+common.FenceAccessTokenEndpoint).
			WithHeader(common.HeaderContentType, common.MIMEApplicationJSON).
			WithBody(bodyBytes),
	)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("failed to refresh token, status: " + strconv.Itoa(resp.StatusCode))
	}

	var result common.AccessTokenStruct
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return errors.New("failed to parse token response: " + err.Error())
	}

	if result.AccessToken == "" {
		return errors.New("empty access token in response")
	}

	profileConfig.AccessToken = result.AccessToken
	return nil
}

// Todo: why isn't this calld in every fence response that has a body ? why is this seperated out
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

	res, err := f.DoAuthenticatedRequest(
		profileConfig,
		&req.RequestBuilder{
			Url:    common.ShepherdVersionEndpoint,
			Method: http.MethodGet,
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
	return false, fmt.Errorf("Shepherd is enabled, but %v does not have correct Shepherd version. (Need Shepherd version >=%v, got %v)", profileConfig.APIEndpoint, minVer, ver)
}

func (f *Functions) DoAuthenticatedRequest(
	cred *conf.Credential,
	rb *req.RequestBuilder,
) (*http.Response, error) {
	if cred.APIEndpoint == "" {
		return nil, errors.New("APIEndpoint is missing in credential")
	}

	baseURL, err := url.Parse(cred.APIEndpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid APIEndpoint URL: %w", err)
	}
	rb.Url = baseURL.ResolveReference(&url.URL{Path: rb.Url}).String()

	// Define refresh callback (combines NewAccessToken + Save)
	refreshCallback := func(c *conf.Credential) error {
		if err := f.NewAccessToken(c); err != nil {
			return err
		}
		return f.Config.Save(c)
	}
	return f.Request.DoAuthenticated(rb, cred, refreshCallback)
}

func (f *Functions) CheckPrivileges(profileConfig *conf.Credential) (map[string]any, error) {
	/*
	   Return user privileges from specified profile
	*/
	var err error
	var data map[string]any

	resp, err := f.DoAuthenticatedRequest(profileConfig,
		&req.RequestBuilder{
			Url:    common.FenceUserEndpoint,
			Method: http.MethodGet},
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

func (f *Functions) DeleteRecord(profileConfig *conf.Credential, guid string) (string, error) {
	endpoint := common.FenceDataEndpoint + "/" + guid
	hasShepherd, err := f.CheckForShepherdAPI(profileConfig)
	if err != nil {
		f.Logger.Printf("WARNING: Error checking Shepherd API: %v. Falling back to Fence.\n", err)
	} else if hasShepherd {
		endpoint = common.ShepherdEndpoint + "/objects/" + guid
	}

	resp, err := f.DoAuthenticatedRequest(profileConfig,
		&req.RequestBuilder{
			Url:    endpoint,
			Method: http.MethodDelete},
	)
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
		err = f.NewAccessToken(cred)
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
