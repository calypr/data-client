package tests

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/calypr/data-client/client/api"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/download"
	"github.com/calypr/data-client/client/mocks"
	req "github.com/calypr/data-client/client/request"
	"github.com/calypr/data-client/client/upload"
	"go.uber.org/mock/gomock"
)

func TestGetDownloadResponse_withShepherd(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFilename := "test-file"
	mockDownloadURL := "https://example.com/example.pfb"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)

	// Mock credential
	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()

	// Shepherd is deployed
	mockGen3.EXPECT().
		CheckForShepherdAPI(gomock.Any()).
		Return(true, nil)

	// Shepherd download URL response
	downloadURLBody := fmt.Sprintf(`{"url": "%s"}`, mockDownloadURL)
	shepherdResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(downloadURLBody)),
	}

	// Expect DoAuthenticatedRequest to Shepherd /objects/{guid}/download
	mockGen3.EXPECT().
		DoAuthenticatedRequest(gomock.Any(), gomock.Any()).
		DoAndReturn(func(cred *conf.Credential, rb *req.RequestBuilder) (*http.Response, error) {
			if !strings.HasSuffix(rb.Url, "/objects/"+testGUID+"/download") {
				t.Errorf("Expected Shepherd download URL request, got %s", rb.Url)
			}
			return shepherdResp, nil
		})

	// ParseFenceURLResponse to extract URL
	mockGen3.EXPECT().
		ParseFenceURLResponse(shepherdResp).
		Return(api.FenceResponse{URL: mockDownloadURL}, nil)

	// We assume the implementation uses http.Client directly for presigned URLs (common pattern)
	// So no mock needed here unless you inject an HTTP client — this part may be unmocked.
	// If you have a mockable HTTP doer, adjust accordingly.

	mockFDRObj := common.FileDownloadResponseObject{
		Filename: testFilename,
		GUID:     testGUID,
		Range:    0,
	}

	err := download.GetDownloadResponse(mockGen3, &mockFDRObj, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if mockFDRObj.URL != mockDownloadURL {
		t.Errorf("Wanted URL %s, got %s", mockDownloadURL, mockFDRObj.URL)
	}

	// Note: Response may be fetched outside the interface (direct http.Get), so this check might not work unless injected.
	// If you want to fully mock it, consider injecting a downloader.
}

func TestGetDownloadResponse_noShepherd(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFilename := "test-file"
	mockDownloadURL := "https://example.com/example.pfb"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()

	// No Shepherd
	mockGen3.EXPECT().
		CheckForShepherdAPI(gomock.Any()).
		Return(false, nil)

	// Fence returns presigned URL
	fenceResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"url": "%s"}`, mockDownloadURL))),
	}

	mockGen3.EXPECT().
		DoAuthenticatedRequest(gomock.Any(), gomock.Any()).
		Return(fenceResp, nil)

	mockGen3.EXPECT().
		ParseFenceURLResponse(fenceResp).
		Return(api.FenceResponse{URL: mockDownloadURL}, nil)

	mockFDRObj := common.FileDownloadResponseObject{
		Filename: testFilename,
		GUID:     testGUID,
		Range:    0,
	}

	err := download.GetDownloadResponse(mockGen3, &mockFDRObj, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if mockFDRObj.URL != mockDownloadURL {
		t.Errorf("Wanted URL %s, got %s", mockDownloadURL, mockFDRObj.URL)
	}
}

func TestGeneratePresignedURL_noShepherd(t *testing.T) {
	testFilename := "test-file"
	testBucketname := "test-bucket"
	mockPresignedURL := "https://example.com/example.pfb"
	mockGUID := "000000-0000000-0000000-000000"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()

	// No Shepherd
	mockGen3.EXPECT().
		CheckForShepherdAPI(gomock.Any()).
		Return(false, nil)

	// Fence upload endpoint response
	fenceResp := &http.Response{
		StatusCode: 200,
		Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
			`{"url": "%s", "guid": "%s"}`, mockPresignedURL, mockGUID,
		))),
	}

	mockGen3.EXPECT().
		DoAuthenticatedRequest(gomock.Any(), gomock.Any()).
		Return(fenceResp, nil)

	mockGen3.EXPECT().
		ParseFenceURLResponse(fenceResp).
		Return(api.FenceResponse{
			URL:  mockPresignedURL,
			GUID: mockGUID,
		}, nil)

	resp, err := upload.GeneratePresignedURL(mockGen3, testFilename, common.FileMetadata{}, testBucketname)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.URL != mockPresignedURL {
		t.Errorf("Wanted URL %s, got %s", mockPresignedURL, resp.URL)
	}
	if resp.GUID != mockGUID {
		t.Errorf("Wanted GUID %s, got %s", mockGUID, resp.GUID)
	}
}

func TestGeneratePresignedURL_withShepherd(t *testing.T) {
	testFilename := "test-file"
	testBucketname := "test-bucket"
	mockPresignedURL := "https://example.com/example.pfb"
	mockGUID := "000000-0000000-0000000-000000"

	testMetadata := common.FileMetadata{
		Aliases:  []string{"test-alias-1", "test-alias-2"},
		Authz:    []string{"authz-resource-1", "authz-resource-2"},
		Metadata: map[string]any{"arbitrary": "metadata"},
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()

	// Shepherd is deployed
	mockGen3.EXPECT().
		CheckForShepherdAPI(gomock.Any()).
		Return(true, nil)

	// Shepherd returns GUID and upload_url
	shepherdResp := &http.Response{
		StatusCode: 201,
		Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
			`{"guid": "%s", "upload_url": "%s"}`, mockGUID, mockPresignedURL,
		))),
	}

	mockGen3.EXPECT().
		DoAuthenticatedRequest(gomock.Any(), gomock.Any()).
		DoAndReturn(func(cred *conf.Credential, rb *req.RequestBuilder) (*http.Response, error) {
			if rb.Method != "POST" || !strings.HasSuffix(rb.Url, "/objects") {
				t.Errorf("Expected POST to /objects, got %s %s", rb.Method, rb.Url)
			}
			// Optionally validate body here if needed
			return shepherdResp, nil
		})

	respObj, err := upload.GeneratePresignedURL(mockGen3, testFilename, testMetadata, testBucketname)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if respObj.URL != mockPresignedURL {
		t.Errorf("Wanted URL %s, got %s", mockPresignedURL, respObj.URL)
	}
	if respObj.GUID != mockGUID {
		t.Errorf("Wanted GUID %s, got %s", mockGUID, respObj.GUID)
	}
}
