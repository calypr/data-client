package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/calypr/data-client/download"
	drs "github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/mocks"
	"go.uber.org/mock/gomock"
)

func Test_askGen3ForFileInfo_withShepherd(t *testing.T) {
	testGUID := "000000-0000000-0000000-000000"
	testFileName := "test-file"
	testFileSize := int64(120)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockIndexd := mocks.NewMockDrsClient(mockCtrl)

	// New behavior: tries GetObjectByHash first
	mockIndexd.EXPECT().
		GetObjectByHash(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("not a hash"))

	mockIndexd.EXPECT().
		GetObject(gomock.Any(), testGUID).
		Return(&drs.DRSObject{Id: testGUID, Name: testFileName, Size: testFileSize}, nil)

	logger := logs.NewGen3Logger(nil, "", "test")

	skipped := []download.RenamedOrSkippedFileInfo{}
	info, err := download.GetFileInfo(context.Background(), mockIndexd, logger, testGUID, "", "", "original", true, &skipped)
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

	mockIndexd := mocks.NewMockDrsClient(mockCtrl)

	// New behavior: tries GetObjectByHash first
	mockIndexd.EXPECT().
		GetObjectByHash(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("not a hash"))

	mockIndexd.EXPECT().
		GetObject(gomock.Any(), testGUID).
		Return(nil, fmt.Errorf("Indexd error"))

	logger := logs.NewGen3Logger(nil, "", "test")

	skipped := []download.RenamedOrSkippedFileInfo{}
	info, err := download.GetFileInfo(context.Background(), mockIndexd, logger, testGUID, "", "", "original", true, &skipped)
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

	mockIndexd := mocks.NewMockDrsClient(mockCtrl)

	// New behavior: tries GetObjectByHash first
	mockIndexd.EXPECT().
		GetObjectByHash(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("not a hash"))

	mockIndexd.EXPECT().
		GetObject(gomock.Any(), testGUID).
		Return(&drs.DRSObject{Id: testGUID, Name: testFileName, Size: testFileSize}, nil)

	logger := logs.NewGen3Logger(nil, "", "test")

	skipped := []download.RenamedOrSkippedFileInfo{}
	info, err := download.GetFileInfo(context.Background(), mockIndexd, logger, testGUID, "", "", "original", true, &skipped)
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
