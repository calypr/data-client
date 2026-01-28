package tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/download"
	"github.com/calypr/data-client/fence"
	"github.com/calypr/data-client/mocks"
	"github.com/calypr/data-client/upload"
	"go.uber.org/mock/gomock"
)

func TestGetDownloadResponse_withShepherd(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFilename := "test-file"
	mockDownloadURL := "https://example.com/example.pfb"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockFence := mocks.NewMockFenceInterface(mockCtrl)

	// Mock credential
	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()
	mockGen3.EXPECT().Fence().Return(mockFence).AnyTimes()

	mockFence.EXPECT().
		GetDownloadPresignedUrl(gomock.Any(), testGUID, "").
		Return(mockDownloadURL, nil)

	// Mock successful response from the presigned URL
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("content")),
	}
	mockFence.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		Return(mockResp, nil)

	mockFDRObj := common.FileDownloadResponseObject{
		Filename: testFilename,
		GUID:     testGUID,
		Range:    0,
	}

	err := download.GetDownloadResponse(context.Background(), mockGen3, &mockFDRObj, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if mockFDRObj.PresignedURL != mockDownloadURL {
		t.Errorf("Wanted URL %s, got %s", mockDownloadURL, mockFDRObj.PresignedURL)
	}
}

func TestGetDownloadResponse_noShepherd(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFilename := "test-file"
	mockDownloadURL := "https://example.com/example.pfb"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockFence := mocks.NewMockFenceInterface(mockCtrl)

	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()
	mockGen3.EXPECT().Fence().Return(mockFence).AnyTimes()

	mockFence.EXPECT().
		GetDownloadPresignedUrl(gomock.Any(), testGUID, "").
		Return(mockDownloadURL, nil)

	// Mock successful response
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("content")),
	}
	mockFence.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		Return(mockResp, nil)

	mockFDRObj := common.FileDownloadResponseObject{
		Filename: testFilename,
		GUID:     testGUID,
		Range:    0,
	}

	err := download.GetDownloadResponse(context.Background(), mockGen3, &mockFDRObj, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if mockFDRObj.PresignedURL != mockDownloadURL {
		t.Errorf("Wanted URL %s, got %s", mockDownloadURL, mockFDRObj.PresignedURL)
	}
}

func TestGeneratePresignedUploadURL_noShepherd(t *testing.T) {
	testFilename := "test-file"
	testBucketname := "test-bucket"
	mockPresignedURL := "https://example.com/example.pfb"
	mockGUID := "000000-0000000-0000000-000000"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockFence := mocks.NewMockFenceInterface(mockCtrl)

	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()
	mockGen3.EXPECT().Fence().Return(mockFence).AnyTimes()

	// No Shepherd
	mockFence.EXPECT().
		CheckForShepherdAPI(gomock.Any()).
		Return(false, nil)

	mockFence.EXPECT().
		InitUpload(gomock.Any(), testFilename, testBucketname, "").
		Return(fence.FenceResponse{
			URL:  mockPresignedURL,
			GUID: mockGUID,
		}, nil)

	resp, err := upload.GeneratePresignedUploadURL(context.Background(), mockGen3, testFilename, common.FileMetadata{}, testBucketname)
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

func TestGeneratePresignedUploadURL_withShepherd(t *testing.T) {
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
	mockFence := mocks.NewMockFenceInterface(mockCtrl)

	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{AccessToken: "token"}).AnyTimes()
	mockGen3.EXPECT().Fence().Return(mockFence).AnyTimes()

	// Shepherd is deployed
	mockFence.EXPECT().
		CheckForShepherdAPI(gomock.Any()).
		Return(true, nil)

	// Shepherd returns GUID and upload_url
	shepherdResp := &http.Response{
		StatusCode: 201,
		Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
			`{"guid": "%s", "upload_url": "%s"}`, mockGUID, mockPresignedURL,
		))),
	}

	mockFence.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		Return(shepherdResp, nil)

	respObj, err := upload.GeneratePresignedUploadURL(context.Background(), mockGen3, testFilename, testMetadata, testBucketname)
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
