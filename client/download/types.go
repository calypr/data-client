package download

import (
	"os"

	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
)

type IndexdResponse struct {
	Name string
	Size int64
}
type RenamedOrSkippedFileInfo struct {
	GUID        string
	OldFilename string
	NewFilename string
}

func validateLocalFileStat(
	logger logs.Logger,
	fdr *common.FileDownloadResponseObject,
	filesize int64,
	skipCompleted bool,
) {
	fullPath := fdr.DownloadPath + fdr.Filename

	fi, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No local file → full download, nothing special
			return
		}
		logger.Printf("Error statting local file \"%s\": %s\n", fullPath, err.Error())
		logger.Println("Will attempt full download anyway")
		return
	}

	localSize := fi.Size()

	// User doesn't want to skip completed files → force full overwrite
	if !skipCompleted {
		fdr.Overwrite = true
		return
	}

	// Exact match → skip entirely
	if localSize == filesize {
		fdr.Skip = true
		return
	}

	// Local file larger than expected → overwrite fully (corrupted or different file)
	if localSize > filesize {
		fdr.Overwrite = true
		return
	}

	fdr.Range = localSize
}
