package download

import (
	"os"

	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
)

type RenamedOrSkippedFileInfo struct {
	GUID        string
	OldFilename string
	NewFilename string
}

func validateLocalFileStat(logger logs.Logger, downloadPath string, filename string, filesize int64, skipCompleted bool) common.FileDownloadResponseObject {
	fi, err := os.Stat(downloadPath + filename) // check filename for local existence
	if err != nil {
		if os.IsNotExist(err) {
			return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename} // no local file, normal full length download
		}
		logger.Printf("Error occurred when getting information for file \"%s\": %s\n", downloadPath+filename, err.Error())
		logger.Println("Will try to download the whole file")
		return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename} // errorred when trying to get local FI, normal full length download
	}

	// have existing local file and may want to skip, check more conditions
	if !skipCompleted {
		return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename, Overwrite: true} // not skipping any local files, normal full length download
	}

	localFilesize := fi.Size()
	if localFilesize == filesize {
		return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename, Skip: true} // both filename and filesize matches, consider as completed
	}
	if localFilesize > filesize {
		return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename, Overwrite: true} // local filesize is greater than INDEXD record, overwrite local existing
	}
	// local filesize is less than INDEXD record, try ranged download
	return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename, Range: localFilesize}
}
