package tests

import (
	"bytes"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/calypr/data-client/client/api"
	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/mocks"
	req "github.com/calypr/data-client/client/request"
	"go.uber.org/mock/gomock"
)

func TestDoAuthenticatedRequest_NoProfile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFuncs := mocks.NewMockFunctionInterface(mockCtrl)

	emptyCred := &conf.Credential{}

	// Expect error when credentials are incomplete
	_, err := mockFuncs.DoAuthenticatedRequest(emptyCred, &req.RequestBuilder{
		Url: "/user/data/download/test_uuid",
	})
	if err == nil {
		t.Error("Expected error due to missing credentials, but got nil")
	}
}

func TestDoAuthenticatedRequest_GoodToken(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFuncs := mocks.NewMockFunctionInterface(mockCtrl)

	cred := &conf.Credential{
		APIKey:      "fake_api_key",
		AccessToken: "non_expired_token",
		APIEndpoint: "https://example.com",
	}

	mockedResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(`{"url": "https://signed.url"}`)),
	}

	mockFuncs.EXPECT().
		DoAuthenticatedRequest(cred, gomock.Any()).
		Return(mockedResp, nil).
		Times(1)

	resp, err := mockFuncs.DoAuthenticatedRequest(cred, &req.RequestBuilder{
		Url: "/user/data/download/test_uuid",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestDoAuthenticatedRequest_MissingToken_CreatesNew(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFuncs := mocks.NewMockFunctionInterface(mockCtrl)
	mockConfig := mocks.NewMockManagerInterface(mockCtrl)

	// Assuming Functions struct has both Config and Functions fields
	testFunction := &api.Functions{
		Config: mockConfig,
	}

	cred := &conf.Credential{
		APIKey:      "fake_api_key",
		AccessToken: "", // empty → should trigger token creation
		APIEndpoint: "https://example.com",
	}

	mockedResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(`{"url": "https://signed.url"}`)),
	}

	// Expect Save to be called if new token is generated and saved
	mockConfig.EXPECT().Save(cred).AnyTimes()

	mockFuncs.EXPECT().
		DoAuthenticatedRequest(cred, gomock.Any()).
		Return(mockedResp, nil).
		Times(1)

	_, err := testFunction.DoAuthenticatedRequest(cred, &req.RequestBuilder{
		Url: "/user/data/download/test_uuid",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestCheckPrivileges_NoProfile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFuncs := mocks.NewMockFunctionInterface(mockCtrl)

	emptyCred := &conf.Credential{}

	_, err := mockFuncs.CheckPrivileges(emptyCred)
	if err == nil {
		t.Error("Expected error when credentials are missing, got nil")
	}
}

func TestCheckPrivileges_NoAccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFuncs := mocks.NewMockFunctionInterface(mockCtrl)

	cred := &conf.Credential{
		APIKey:      "fake_api_key",
		AccessToken: "valid_token",
		APIEndpoint: "https://example.com",
	}

	userResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"project_access": {}}`)),
	}

	mockFuncs.EXPECT().
		DoAuthenticatedRequest(cred, gomock.Any()).
		Return(userResp, nil)

	privileges, err := mockFuncs.CheckPrivileges(cred)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := make(map[string]any)
	if !reflect.DeepEqual(privileges, expected) {
		t.Errorf("Expected empty privileges, got %v", privileges)
	}
}

func TestCheckPrivileges_GrantedAccess_ProjectAccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFuncs := mocks.NewMockFunctionInterface(mockCtrl)

	cred := &conf.Credential{
		APIKey:      "fake_api_key",
		AccessToken: "valid_token",
		APIEndpoint: "https://example.com",
	}

	jsonBody := `{
		"project_access": {
			"test_project": ["read", "create", "read-storage", "update", "delete"]
		}
	}`

	userResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(jsonBody)),
	}

	mockFuncs.EXPECT().
		DoAuthenticatedRequest(cred, gomock.Any()).
		Return(userResp, nil)

	privileges, err := mockFuncs.CheckPrivileges(cred)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := map[string]any{
		"test_project": []any{"read", "create", "read-storage", "update", "delete"},
	}

	if !reflect.DeepEqual(privileges, expected) {
		t.Errorf("Privileges mismatch.\nExpected: %v\nGot:      %v", expected, privileges)
	}
}

func TestCheckPrivileges_GrantedAccess_AuthzTakesPrecedence(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFuncs := mocks.NewMockFunctionInterface(mockCtrl)

	cred := &conf.Credential{
		APIKey:      "fake_api_key",
		AccessToken: "valid_token",
		APIEndpoint: "https://example.com",
	}

	jsonBody := `{
		"authz": {
			"test_project": [
				{"method": "create", "service": "*"},
				{"method": "delete", "service": "*"},
				{"method": "read", "service": "*"},
				{"method": "read-storage", "service": "*"},
				{"method": "update", "service": "*"},
				{"method": "upload", "service": "*"}
			]
		},
		"project_access": {
			"test_project": ["read", "create", "read-storage", "update", "delete"]
		}
	}`

	userResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(jsonBody)),
	}

	mockFuncs.EXPECT().
		DoAuthenticatedRequest(cred, gomock.Any()).
		Return(userResp, nil)

	privileges, err := mockFuncs.CheckPrivileges(cred)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := map[string]any{
		"test_project": []any{
			map[string]any{"method": "create", "service": "*"},
			map[string]any{"method": "delete", "service": "*"},
			map[string]any{"method": "read", "service": "*"},
			map[string]any{"method": "read-storage", "service": "*"},
			map[string]any{"method": "update", "service": "*"},
			map[string]any{"method": "upload", "service": "*"},
		},
	}

	if !reflect.DeepEqual(privileges, expected) {
		t.Errorf("Authz privileges should take precedence.\nExpected: %v\nGot:      %v", expected, privileges)
	}
}
