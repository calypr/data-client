package tests

import (
	"bytes"
	"fmt"
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

func TestDoRequestWithSignedHeaderNoProfile(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockConfig := mocks.NewMockManagerInterface(mockCtrl)
	testFunction := &api.Functions{Config: mockConfig}

	profileConfig := conf.Credential{KeyID: "", APIKey: "", AccessToken: "", APIEndpoint: ""}

	_, err := testFunction.DoAuthenticatedRequest(&profileConfig,
		&req.RequestBuilder{
			Url: "/user/data/download/test_uuid",
		})

	if err == nil {
		t.Fail()
	}
}

func TestDoRequestWithSignedHeaderGoodToken(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockConfig := mocks.NewMockManagerInterface(mockCtrl)
	mockRequest := mocks.NewMockRequestInterface(mockCtrl)
	testFunction := &api.Functions{Config: mockConfig, Request: mockRequest}

	profileConfig := conf.Credential{Profile: "test", KeyID: "", APIKey: "fake_api_key", AccessToken: "non_expired_token", APIEndpoint: "http://www.test.com"}
	mockedResp := &http.Response{
		Body:       io.NopCloser(bytes.NewBufferString("{\"url\": \"http://www.test.com/user/data/download/test_uuid\"}")),
		StatusCode: 200,
	}

	mockRequest.EXPECT().
		DoAuthenticated(gomock.Any(), &profileConfig, gomock.Any()).
		Return(mockedResp, nil).
		Times(1)

	_, err := testFunction.DoAuthenticatedRequest(&profileConfig,
		&req.RequestBuilder{Url: "/user/data/download/test_uuid"},
	)

	if err != nil {
		t.Fail()
	}
}

func TestDoRequestWithSignedHeaderCreateNewToken(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockConfig := mocks.NewMockManagerInterface(mockCtrl)
	mockRequest := mocks.NewMockRequestInterface(mockCtrl)
	testFunction := &api.Functions{Config: mockConfig, Request: mockRequest}

	profileConfig := &conf.Credential{KeyID: "", APIKey: "fake_api_key", AccessToken: "", APIEndpoint: "http://www.test.com"}
	mockedResp := &http.Response{
		Body:       io.NopCloser(bytes.NewBufferString("{\"url\": \"www.test.com/user/data/download/\"}")),
		StatusCode: 200,
	}

	// The new interface expects DoAuthenticated to handle the logic.
	// If your code still calls "Save", keep that expectation.
	mockConfig.EXPECT().Save(profileConfig).AnyTimes()

	mockRequest.EXPECT().
		DoAuthenticated(gomock.Any(), profileConfig, gomock.Any()).
		Return(mockedResp, nil).
		Times(1)

	_, err := testFunction.DoAuthenticatedRequest(profileConfig,
		&req.RequestBuilder{Url: "/user/data/download/test_uuid"})

	if err != nil {
		t.Fail()
	}
}
func TestDoRequestWithSignedHeaderRefreshToken(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockConfig := mocks.NewMockManagerInterface(mockCtrl)
	mockRequest := mocks.NewMockRequestInterface(mockCtrl)
	testFunction := &api.Functions{Config: mockConfig, Request: mockRequest}

	profileConfig := &conf.Credential{KeyID: "", APIKey: "fake_api_key", AccessToken: "expired_token", APIEndpoint: "http://www.test.com"}
	mockedResp := &http.Response{
		Body:       io.NopCloser(bytes.NewBufferString("{\"url\": \"www.test.com/user/data/download/\"}")),
		StatusCode: 401,
	}

	mockConfig.EXPECT().Save(profileConfig).Times(1)
	mockRequest.EXPECT().RequestNewAccessToken("http://www.test.com/user/credentials/api/access_token", profileConfig).Return(nil).Times(1)
	mockRequest.EXPECT().MakeARequest(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(mockedResp, nil).Times(2)

	_, err := testFunction.DoAuthenticatedRequest(profileConfig,
		&req.RequestBuilder{Url: "/user/data/download/test_uuid"},
	)

	if err != nil && !strings.Contains(err.Error(), "401") {
		t.Fail()
	}

}

func TestCheckPrivilegesNoProfile(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockConfig := mocks.NewMockManagerInterface(mockCtrl)
	testFunction := &api.Functions{Config: mockConfig}

	profileConfig := conf.Credential{KeyID: "", APIKey: "", AccessToken: "", APIEndpoint: ""}

	_, err := testFunction.CheckPrivileges(&profileConfig)

	if err == nil {
		t.Errorf("Expected an error on missing credentials in configuration, but not received")
	}
}

func TestCheckPrivilegesNoAccess(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockConfig := mocks.NewMockManagerInterface(mockCtrl)
	mockRequest := mocks.NewMockRequestInterface(mockCtrl)
	testFunction := &api.Functions{Config: mockConfig, Request: mockRequest}

	profileConfig := conf.Credential{KeyID: "", APIKey: "fake_api_key", AccessToken: "non_expired_token", APIEndpoint: "http://www.test.com"}
	mockedResp := &http.Response{
		Body:       io.NopCloser(bytes.NewBufferString("{\"project_access\": {}}")),
		StatusCode: 200,
	}

	builder := &req.RequestBuilder{}
	mockRequest.EXPECT().New("GET", "http://www.test.com/user/user").Return(builder)

	receivedAccess, err := testFunction.CheckPrivileges(&profileConfig)
	mockRequest.EXPECT().Do(builder).Return(mockedResp, nil)

	expectedAccess := make(map[string]any)

	if err != nil {
		t.Errorf("Expected no errors, received an error \"%v\"", err)
	} else if !reflect.DeepEqual(receivedAccess, expectedAccess) {
		t.Errorf("Expected no user access, received %v", receivedAccess)
	}
}

func TestCheckPrivilegesGrantedAccess(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockConfig := mocks.NewMockManagerInterface(mockCtrl)
	mockRequest := mocks.NewMockRequestInterface(mockCtrl)
	testFunction := &api.Functions{Config: mockConfig, Request: mockRequest}

	profileConfig := conf.Credential{KeyID: "", APIKey: "fake_api_key", AccessToken: "non_expired_token", APIEndpoint: "http://www.test.com"}

	grantedAccessJSON := `{
		"project_access":
			{
				"test_project": ["read", "create","read-storage","update","delete"]
			}
		}`

	mockedResp := &http.Response{
		Body:       io.NopCloser(bytes.NewBufferString(grantedAccessJSON)),
		StatusCode: 200,
	}

	builder := &req.RequestBuilder{}
	mockRequest.EXPECT().
		New("GET", "http://www.test.com/user/user").
		Return(builder).
		Times(1)

	// 2. Mock the Do execution using that builder
	mockRequest.EXPECT().
		Do(builder).
		Return(mockedResp, nil).
		Times(1)

	expectedAccess, err := testFunction.CheckPrivileges(&profileConfig)

	receivedAccess := make(map[string]any)
	receivedAccess["test_project"] = []any{
		"read",
		"create",
		"read-storage",
		"update",
		"delete"}

	if err != nil {
		t.Errorf("Expected no errors, received an error \"%v\"", err)
	} else if !reflect.DeepEqual(expectedAccess, receivedAccess) {
		t.Errorf(`Expected user access and received user access are not the same.
        Expected: %v
        Received: %v`, expectedAccess, receivedAccess)
	}
}

// If both `authz` and `project_access` section exists, `authz` takes precedence
func TestCheckPrivilegesGrantedAccessAuthz(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockConfig := mocks.NewMockManagerInterface(mockCtrl)
	mockRequest := mocks.NewMockRequestInterface(mockCtrl)
	testFunction := &api.Functions{Config: mockConfig, Request: mockRequest}

	profileConfig := conf.Credential{
		KeyID:       "",
		APIKey:      "fake_api_key",
		AccessToken: "non_expired_token",
		APIEndpoint: "http://www.test.com",
	}

	grantedAccessJSON := `{
        "authz": {
            "test_project":[
                {"method":"create", "service":"*"},
                {"method":"delete", "service":"*"},
                {"method":"read", "service":"*"},
                {"method":"read-storage", "service":"*"},
                {"method":"update", "service":"*"},
                {"method":"upload", "service":"*"}
            ]
        },
        "project_access": {
            "test_project": ["read", "create","read-storage","update","delete"]
        }
    }`

	mockedResp := &http.Response{
		Body:       io.NopCloser(bytes.NewBufferString(grantedAccessJSON)),
		StatusCode: 200,
	}

	// --- NEW MOCK EXPECTATIONS ---
	// 1. Define a dummy builder that New will return and Do will receive
	builder := &req.RequestBuilder{}

	// 2. Expect New to be called to construct the request
	mockRequest.EXPECT().
		New("GET", "http://www.test.com/user/user").
		Return(builder).
		Times(1)

	// 3. Expect Do to be called with that same builder
	mockRequest.EXPECT().
		Do(builder).
		Return(mockedResp, nil).
		Times(1)
	// -----------------------------

	// Execute the function under test
	expectedAccess, err := testFunction.CheckPrivileges(&profileConfig)

	// Define what we expect the function to return
	receivedAccess := make(map[string]any)
	receivedAccess["test_project"] = []map[string]any{
		{"method": "create", "service": "*"},
		{"method": "delete", "service": "*"},
		{"method": "read", "service": "*"},
		{"method": "read-storage", "service": "*"},
		{"method": "update", "service": "*"},
		{"method": "upload", "service": "*"},
	}

	// Assertions
	if err != nil {
		t.Errorf("Expected no errors, received an error \"%v\"", err)
	} else if fmt.Sprint(expectedAccess) != fmt.Sprint(receivedAccess) {
		t.Errorf(`Expected user access and received user access are not the same.
        Expected: %v
        Received: %v`, expectedAccess, receivedAccess)
	}
}
