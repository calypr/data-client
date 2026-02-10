package tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/calypr/data-client/backend/gen3"
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/download"
	drs "github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/mocks"
	req "github.com/calypr/data-client/request"
	"go.uber.org/mock/gomock"
)

func Test_askGen3ForFileInfo_withShepherd(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFileName := "test-file"
	testFileSize := int64(120)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockFence := mocks.NewMockFenceInterface(mockCtrl)

	// Expect credential access
	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()
	mockGen3.EXPECT().Fence().Return(mockFence).AnyTimes()

	// Shepherd is available
	mockFence.EXPECT().
		CheckForShepherdAPI(gomock.Any()).
		Return(true, nil)

	// Mock successful Shepherd response
	testBody := `{
		"record": {
			"file_name": "test-file",
			"size": 120,
			"did": "000000-0000000-0000000-000000"
		}
	}`
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(testBody)),
	}

	// Expect request to Shepherd
	mockGen3.EXPECT().
		New(gomock.Any(), gomock.Any()).
		Return(&req.RequestBuilder{Url: "/objects/" + testGUID}).
		AnyTimes()

	mockGen3.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx any, rb *req.RequestBuilder) (*http.Response, error) {
			return resp, nil
		})

	// Optional: logger
	mockGen3.EXPECT().Logger().Return(logs.NewGen3Logger(nil, "", "test")).AnyTimes()

	skipped := []download.RenamedOrSkippedFileInfo{}
	bk := gen3.NewGen3Backend(mockGen3)
	info, err := download.GetFileInfo(context.Background(), bk, testGUID, "", "", "original", true, &skipped)
	if err != nil {
		t.Error(err)
	}

	if info.Name != testFileName {
		t.Errorf("Wanted filename %v, got %v", testFileName, info.Name)
	}
	if info.Size != testFileSize {
		t.Errorf("Wanted filesize %v, got %v", testFileSize, info.Size)
	}
	if len(skipped) != 0 {
		t.Errorf("Expected no skipped files, got %v", skipped)
	}
}

func Test_askGen3ForFileInfo_withShepherd_shepherdError(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockFence := mocks.NewMockFenceInterface(mockCtrl)

	dummyCred := &conf.Credential{}
	mockGen3.EXPECT().GetCredential().Return(dummyCred).AnyTimes()
	mockGen3.EXPECT().Fence().Return(mockFence).AnyTimes()

	// 1. Shepherd is available
	mockFence.EXPECT().
		CheckForShepherdAPI(gomock.Any()).
		Return(true, nil).
		Times(1)

	// 2. Shepherd request fails → triggers fallback to Indexd
	mockGen3.EXPECT().
		New(gomock.Any(), gomock.Any()).
		Return(&req.RequestBuilder{Url: "/objects/" + testGUID}).
		AnyTimes()

	mockGen3.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("Shepherd error")).
		Times(1) // only the Shepherd call

	// 3. Fallback: Indexd request
	mockIndexd := mocks.NewMockIndexdInterface(mockCtrl)
	mockGen3.EXPECT().Indexd().Return(mockIndexd).AnyTimes()
	mockIndexd.EXPECT().
		GetObject(gomock.Any(), testGUID).
		Return(nil, fmt.Errorf("Indexd error"))

	// Logger
	mockGen3.EXPECT().
		Logger().
		Return(logs.NewGen3Logger(nil, "", "test")).
		AnyTimes()

	skipped := []download.RenamedOrSkippedFileInfo{}
	bk := gen3.NewGen3Backend(mockGen3)
	info, err := download.GetFileInfo(context.Background(), bk, testGUID, "", "", "original", true, &skipped)
	if err != nil {
		t.Fatal(err)
	}

	if info == nil {
		t.Fatal("AskGen3ForFileInfo returned nil when both Shepherd and Indexd failed. Expected fallback FileInfo with Name = GUID")
	}

	if info.Name != testGUID {
		t.Errorf("Wanted fallback filename %v, got %v", testGUID, info.Name)
	}

	if len(skipped) != 1 {
		t.Errorf("Expected exactly 1 skipped file, got %d", len(skipped))
	} else if skipped[0].GUID != testGUID || skipped[0].NewFilename != testGUID {
		t.Errorf("Skipped entry mismatch: %+v", skipped[0])
	}
}

func Test_askGen3ForFileInfo_noShepherd(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFileName := "test-file"
	testFileSize := int64(120)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockFence := mocks.NewMockFenceInterface(mockCtrl)

	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()
	mockGen3.EXPECT().Fence().Return(mockFence).AnyTimes()

	// No Shepherd
	mockFence.EXPECT().CheckForShepherdAPI(gomock.Any()).Return(false, nil)

	mockIndexd := mocks.NewMockIndexdInterface(mockCtrl)
	mockGen3.EXPECT().Indexd().Return(mockIndexd).AnyTimes()
	mockIndexd.EXPECT().
		GetObject(gomock.Any(), testGUID).
		Return(&drs.DRSObject{Id: testGUID, Name: testFileName, Size: testFileSize}, nil)

	mockGen3.EXPECT().Logger().Return(logs.NewGen3Logger(nil, "", "test")).AnyTimes()

	skipped := []download.RenamedOrSkippedFileInfo{}
	bk := gen3.NewGen3Backend(mockGen3)
	info, err := download.GetFileInfo(context.Background(), bk, testGUID, "", "", "original", true, &skipped)
	if err != nil {
		t.Fatal(err)
	}

	if info.Name != testFileName {
		t.Errorf("Wanted filename %v, got %v", testFileName, info.Name)
	}
	if info.Size != testFileSize {
		t.Errorf("Wanted filesize %v, got %v", testFileSize, info.Size)
	}
}
