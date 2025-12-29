package tests

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/calypr/data-client/client/api"
	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/download"
	"github.com/calypr/data-client/client/logs"
	"github.com/calypr/data-client/client/mocks"
	req "github.com/calypr/data-client/client/request"
	"go.uber.org/mock/gomock"
)

func Test_askGen3ForFileInfo_withShepherd(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFileName := "test-file"
	testFileSize := int64(120)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)

	// Expect credential access
	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()

	// Shepherd is available
	mockGen3.EXPECT().
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

	// Expect authenticated request to Shepherd
	mockGen3.EXPECT().
		DoAuthenticatedRequest(gomock.Any(), gomock.Any()).
		DoAndReturn(func(cred *conf.Credential, rb *req.RequestBuilder) (*http.Response, error) {
			if !strings.HasSuffix(rb.Url, "/objects/"+testGUID) {
				t.Errorf("Expected request to Shepherd objects endpoint, got %s", rb.Url)
			}
			return resp, nil
		})

	// Optional: logger
	mockGen3.EXPECT().Logger().Return(logs.NewTeeLogger("", "test", os.Stdout)).AnyTimes()

	skipped := []download.RenamedOrSkippedFileInfo{}
	info, err := download.AskGen3ForFileInfo(mockGen3, testGUID, "", "", "original", true, &skipped)
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

	dummyCred := &conf.Credential{}
	mockGen3.EXPECT().GetCredential().Return(dummyCred).AnyTimes()

	// 1. Shepherd is available
	mockGen3.EXPECT().
		CheckForShepherdAPI(gomock.Any()).
		Return(true, nil).
		Times(1)

	// 2. Shepherd request fails → triggers fallback to Indexd
	mockGen3.EXPECT().
		DoAuthenticatedRequest(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("Shepherd error")).
		Times(1) // only the Shepherd call

	// 3. Fallback: Indexd request also fails (we want to test error handling)
	mockGen3.EXPECT().
		DoAuthenticatedRequest(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("Indexd error")).
		Times(1)

	// Optional: if it tries to parse nil response from Indexd
	mockGen3.EXPECT().
		ParseFenceURLResponse(gomock.Nil()).
		Return(api.FenceResponse{}, fmt.Errorf("no response")).
		AnyTimes()

	// Logger
	mockGen3.EXPECT().
		Logger().
		Return(logs.NewTeeLogger("", "test", os.Stdout)).
		AnyTimes()

	skipped := []download.RenamedOrSkippedFileInfo{}
	info, err := download.AskGen3ForFileInfo(mockGen3, testGUID, "", "", "original", true, &skipped)
	if err != nil {
		t.Fatal(err)
	}

	// Critical fix: check for nil first
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
	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()

	// No Shepherd
	mockGen3.EXPECT().CheckForShepherdAPI(gomock.Any()).Return(false, nil)

	// Indexd returns parsed FenceResponse
	mockGen3.EXPECT().
		ParseFenceURLResponse(gomock.Any()).
		Return(api.FenceResponse{FileName: testFileName, Size: testFileSize}, nil)

	// DoAuthenticatedRequest called for indexd
	mockGen3.EXPECT().
		DoAuthenticatedRequest(gomock.Any(), gomock.Any()).
		Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil)

	mockGen3.EXPECT().Logger().Return(logs.NewTeeLogger("", "test", os.Stdout)).AnyTimes()

	skipped := []download.RenamedOrSkippedFileInfo{}
	info, err := download.AskGen3ForFileInfo(mockGen3, testGUID, "", "", "original", true, &skipped)
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

func Test_askGen3ForFileInfo_noShepherd_indexdError(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGen3 := mocks.NewMockGen3Interface(mockCtrl)
	mockGen3.EXPECT().GetCredential().Return(&conf.Credential{}).AnyTimes()
	mockGen3.EXPECT().CheckForShepherdAPI(gomock.Any()).Return(false, nil)

	// Indexd request fails
	mockGen3.EXPECT().
		DoAuthenticatedRequest(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("Indexd error"))

	mockGen3.EXPECT().Logger().Return(logs.NewTeeLogger("", "test", os.Stdout)).AnyTimes()

	skipped := []download.RenamedOrSkippedFileInfo{}
	info, err := download.AskGen3ForFileInfo(mockGen3, testGUID, "", "", "original", true, &skipped)
	if err != nil {
		t.Fatal(err)
	}

	if info.Name != testGUID {
		t.Errorf("Wanted fallback filename %v, got %v", testGUID, info.Name)
	}
	if len(skipped) != 1 || skipped[0].GUID != testGUID {
		t.Errorf("Expected skipped entry for GUID: %v", skipped)
	}
}
