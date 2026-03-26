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
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/mocks"
	"github.com/calypr/data-client/request"
	"github.com/calypr/data-client/transfer"
	gen3signer "github.com/calypr/data-client/transfer/signer/gen3"
	"github.com/calypr/data-client/upload"
	"go.uber.org/mock/gomock"
)

type staticCredentialsManager struct {
	cred *conf.Credential
}

func (s *staticCredentialsManager) Current() *conf.Credential { return s.cred }
func (s *staticCredentialsManager) Export(ctx context.Context, cred *conf.Credential) error {
	return nil
}

func TestGetDownloadResponse_withShepherd(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFilename := "test-file"
	mockDownloadURL := "https://example.com/example.pfb"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockFence := mocks.NewMockFenceInterface(mockCtrl)
	mockDrs := mocks.NewMockDrsClient(mockCtrl)

	// Mock credential
	mockGen3.EXPECT().Credentials().Return(&staticCredentialsManager{cred: &conf.Credential{}}).AnyTimes()
	mockGen3.EXPECT().FenceClient().Return(mockFence).AnyTimes()
	mockGen3.EXPECT().DRSClient().Return(mockDrs).AnyTimes()
	mockGen3.EXPECT().Logger().Return(logs.NewGen3Logger(nil, "", "test")).AnyTimes()

	mockFence.EXPECT().
		GetDownloadPresignedUrl(gomock.Any(), testGUID, "").
		Return(mockDownloadURL, nil)

	mockGen3.EXPECT().
		New(http.MethodGet, mockDownloadURL).
		Return(&request.RequestBuilder{
			Method:  http.MethodGet,
			Url:     mockDownloadURL,
			Headers: make(map[string]string),
		}).
		AnyTimes()

	// Mock successful response from the presigned URL
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("content")),
	}
	mockGen3.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		Return(mockResp, nil)

	mockFDRObj := common.FileDownloadResponseObject{
		Filename: testFilename,
		GUID:     testGUID,
		Range:    0,
	}

	bk := transfer.New(mockGen3, logs.NewGen3Logger(nil, "", "test"), gen3signer.New(mockGen3, &conf.Credential{}, mockDrs, mockFence))
	err := download.GetDownloadResponse(context.Background(), bk, &mockFDRObj, "")
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
	mockDrs := mocks.NewMockDrsClient(mockCtrl)

	mockGen3.EXPECT().Credentials().Return(&staticCredentialsManager{cred: &conf.Credential{}}).AnyTimes()
	mockGen3.EXPECT().FenceClient().Return(mockFence).AnyTimes()
	mockGen3.EXPECT().DRSClient().Return(mockDrs).AnyTimes()
	mockGen3.EXPECT().Logger().Return(logs.NewGen3Logger(nil, "", "test")).AnyTimes()

	mockFence.EXPECT().
		GetDownloadPresignedUrl(gomock.Any(), testGUID, "").
		Return(mockDownloadURL, nil)

	mockGen3.EXPECT().
		New(http.MethodGet, mockDownloadURL).
		Return(&request.RequestBuilder{
			Method:  http.MethodGet,
			Url:     mockDownloadURL,
			Headers: make(map[string]string),
		}).
		AnyTimes()

	// Mock successful response
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("content")),
	}
	mockGen3.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		Return(mockResp, nil)

	mockFDRObj := common.FileDownloadResponseObject{
		Filename: testFilename,
		GUID:     testGUID,
		Range:    0,
	}

	bk := transfer.New(mockGen3, logs.NewGen3Logger(nil, "", "test"), gen3signer.New(mockGen3, &conf.Credential{}, mockDrs, mockFence))
	err := download.GetDownloadResponse(context.Background(), bk, &mockFDRObj, "")
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
	mockDrs := mocks.NewMockDrsClient(mockCtrl)

	mockGen3.EXPECT().Credentials().Return(&staticCredentialsManager{cred: &conf.Credential{}}).AnyTimes()
	mockGen3.EXPECT().FenceClient().Return(mockFence).AnyTimes()
	mockGen3.EXPECT().DRSClient().Return(mockDrs).AnyTimes()
	mockGen3.EXPECT().Logger().Return(logs.NewGen3Logger(nil, "", "test")).AnyTimes()

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

	bk := transfer.New(mockGen3, logs.NewGen3Logger(nil, "", "test"), gen3signer.New(mockGen3, &conf.Credential{}, mockDrs, mockFence))
	resp, err := upload.GeneratePresignedUploadURL(context.Background(), bk, testFilename, common.FileMetadata{}, testBucketname)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.URL != mockPresignedURL {
		t.Errorf("Wanted URL %s, got %s", mockPresignedURL, resp.URL)
	}
	if resp.GUID != "" {
		t.Errorf("Wanted empty GUID, got %s", resp.GUID)
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
	mockDrs := mocks.NewMockDrsClient(mockCtrl)

	mockGen3.EXPECT().Credentials().Return(&staticCredentialsManager{cred: &conf.Credential{AccessToken: "token"}}).AnyTimes()
	mockGen3.EXPECT().FenceClient().Return(mockFence).AnyTimes()
	mockGen3.EXPECT().DRSClient().Return(mockDrs).AnyTimes()
	mockGen3.EXPECT().Logger().Return(logs.NewGen3Logger(nil, "", "test")).AnyTimes()

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

	bk := transfer.New(mockGen3, logs.NewGen3Logger(nil, "", "test"), gen3signer.New(mockGen3, &conf.Credential{AccessToken: "token", APIEndpoint: "https://example.com"}, mockDrs, mockFence))
	respObj, err := upload.GeneratePresignedUploadURL(context.Background(), bk, testFilename, testMetadata, testBucketname)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if respObj.URL != mockPresignedURL {
		t.Errorf("Wanted URL %s, got %s", mockPresignedURL, respObj.URL)
	}
	if respObj.GUID != "" {
		t.Errorf("Wanted empty GUID, got %s", respObj.GUID)
	}
}
