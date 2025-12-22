package tests

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/calypr/data-client/client/api"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/download"
	"github.com/calypr/data-client/client/logs"
	"github.com/calypr/data-client/client/mocks"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"
)

// Add all other methods required by your logs.Logger interface!

// If Shepherd is deployed, attempt to get the filename from the Shepherd API.
func Test_askGen3ForFileInfo_withShepherd(t *testing.T) {
	// -- SETUP --
	testGUID := "000000-0000000-0000000-000000"
	testFileName := "test-file"
	testFileSize := int64(120)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Expect AskGen3ForFileInfo to call shepherd looking for testGUID: respond with a valid file.
	testBody := `{
	"record": {
		"file_name": "test-file",
		"size": 120,
		"did": "000000-0000000-0000000-000000"
	},
	"metadata": {
		"_file_type": "PFB",
		"_resource_paths": ["/open"],
		"_uploader_id": 42,
		"_bucket": "s3://gen3-bucket"
	}
}`
	testResponse := http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(testBody)),
	}
	mockGen3Interface := mocks.NewMockGen3Interface(mockCtrl)
	mockGen3Interface.
		EXPECT().
		CheckForShepherdAPI().
		Return(true, nil)
	mockGen3Interface.
		EXPECT().
		GetResponse(common.ShepherdEndpoint+"/objects/"+testGUID, "GET", "", nil).
		Return("", &testResponse, nil)
	// ----------

	// Expect AskGen3ForFileInfo to return the correct filename and filesize from shepherd.
	fileName, fileSize := download.AskGen3ForFileInfo(mockGen3Interface, testGUID, "", "", "original", true, &[]download.RenamedOrSkippedFileInfo{})
	if fileName != testFileName {
		t.Errorf("Wanted filename %v, got %v", testFileName, fileName)
	}
	if fileSize != testFileSize {
		t.Errorf("Wanted filesize %v, got %v", testFileSize, fileSize)
	}
}

// If there's an error while getting the filename from Shepherd, add the guid
// to *renamedFiles, which tracks which files have errored.
func Test_askGen3ForFileInfo_withShepherd_shepherdError(t *testing.T) {
	// -- SETUP --
	testGUID := "000000-0000000-0000000-000000"
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Expect AskGen3ForFileInfo to call indexd looking for testGUID:
	// Respond with an error.
	mockGen3Interface := mocks.NewMockGen3Interface(mockCtrl)
	mockGen3Interface.
		EXPECT().
		CheckForShepherdAPI().
		Return(true, nil)
	mockGen3Interface.
		EXPECT().
		GetResponse(common.ShepherdEndpoint+"/objects/"+testGUID, "GET", "", nil).
		Return("", nil, fmt.Errorf("Error getting metadata from Shepherd"))
	// ----------

	mockGen3Interface.
		EXPECT().
		Logger().
		Return(logs.NewTeeLogger("", "test", os.Stdout)). // Or your appropriate dummy logger
		AnyTimes()

	// Expect AskGen3ForFileInfo to add this file's GUID to the renamedOrSkippedFiles array.
	skipped := []download.RenamedOrSkippedFileInfo{}
	fileName, _ := download.AskGen3ForFileInfo(mockGen3Interface, testGUID, "", "", "original", true, &skipped)
	expected := download.RenamedOrSkippedFileInfo{GUID: testGUID, OldFilename: "N/A", NewFilename: testGUID}
	if skipped[0] != expected {
		t.Errorf("Wanted skipped files list to contain %v, got %v", expected, skipped)
	}
	// Expect the returned filename to be the file's GUID.
	if fileName != testGUID {
		t.Errorf("Wanted filename %v, got %v", testGUID, fileName)
	}
}

// If Shepherd is not deployed, attempt to get the filename from indexd.
func Test_askGen3ForFileInfo_noShepherd(t *testing.T) {
	// -- SETUP --
	testGUID := "000000-0000000-0000000-000000"
	testFileName := "test-file"
	testFileSize := int64(120)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Expect AskGen3ForFileInfo to call indexd looking for testGUID: respond with a valid file.
	mockGen3Interface := mocks.NewMockGen3Interface(mockCtrl)
	mockGen3Interface.
		EXPECT().
		CheckForShepherdAPI().
		Return(false, nil)
	mockGen3Interface.
		EXPECT().
		DoRequestWithSignedHeader(common.IndexdIndexEndpoint+"/"+testGUID, "", nil).
		Return(api.FenceResponse{FileName: testFileName, Size: testFileSize}, nil)
	// ----------

	mockGen3Interface.
		EXPECT().
		Logger().
		Return(logs.NewTeeLogger("", "test", os.Stdout)). // Or your appropriate dummy logger
		AnyTimes()

	// Expect AskGen3ForFileInfo to return the correct filename and filesize from indexd.
	fileName, fileSize := download.AskGen3ForFileInfo(mockGen3Interface, testGUID, "", "", "original", true, &[]download.RenamedOrSkippedFileInfo{})
	if fileName != testFileName {
		t.Errorf("Wanted filename %v, got %v", testFileName, fileName)
	}
	if fileSize != testFileSize {
		t.Errorf("Wanted filesize %v, got %v", testFileSize, fileSize)
	}
}

// If there's an error while getting the filename from indexd, add the guid
// to *renamedFiles, which tracks which files have errored.
func Test_askGen3ForFileInfo_noShepherd_indexdError(t *testing.T) {
	// -- SETUP --
	testGUID := "000000-0000000-0000000-000000"
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Expect AskGen3ForFileInfo to call indexd looking for testGUID:
	// Respond with an error.
	mockGen3Interface := mocks.NewMockGen3Interface(mockCtrl)
	mockGen3Interface.
		EXPECT().
		CheckForShepherdAPI(mock.Anything).
		Return(false, nil)
	mockGen3Interface.
		EXPECT().
		DoAuthenticatedRequest(common.IndexdIndexEndpoint+"/"+testGUID, "", nil).
		Return(api.FenceResponse{}, fmt.Errorf("Error downloading file from Indexd"))
	// ----------
	mockGen3Interface.
		EXPECT().
		Logger().
		Return(logs.NewTeeLogger("", "test", os.Stdout)). // Or your appropriate dummy logger
		AnyTimes()

	// Expect AskGen3ForFileInfo to add this file's GUID to the renamedOrSkippedFiles array.
	skipped := []download.RenamedOrSkippedFileInfo{}
	fileName, _ := download.AskGen3ForFileInfo(mockGen3Interface, testGUID, "", "", "original", true, &skipped)
	expected := download.RenamedOrSkippedFileInfo{GUID: testGUID, OldFilename: "N/A", NewFilename: testGUID}
	if skipped[0] != expected {
		t.Errorf("Wanted skipped files list to contain %v, got %v", expected, skipped)
	}
	// Expect the returned filename to be the file's GUID.
	if fileName != testGUID {
		t.Errorf("Wanted filename %v, got %v", testGUID, fileName)
	}
}
