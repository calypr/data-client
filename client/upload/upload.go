package upload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
	"github.com/vbauerster/mpb/v8"
)

func UploadSingleFileWrapper(ctx context.Context, profile, bucket, filePath, guid string, progress bool) error {
	logger, closer := logs.New(profile, logs.WithSucceededLog(), logs.WithFailedLog(), logs.WithScoreboard())
	defer closer()

	g3, err := client.NewGen3Interface(
		profile,
		logger,
	)
	if err != nil {
		fmt.Println("HELLO WE HERE: ", err)
		return fmt.Errorf("failed to initialize Gen3 interface: %w", err)
	}

	absPath, err := common.GetAbsolutePath(filePath)
	if err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	fileInfo := common.FileUploadRequestObject{
		FilePath:     absPath,
		Filename:     filepath.Base(absPath),
		GUID:         guid,
		FileMetadata: common.FileMetadata{},
	}

	return MultipartUpload(ctx, g3, fileInfo, progress)
}

// UploadSingleFile handles single-part upload with progress
func UploadSingleFile(ctx context.Context, g3 client.Gen3Interface, req common.FileUploadRequestObject, showProgress bool) error {
	file, err := os.Open(req.FilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	fi, _ := file.Stat()
	if fi.Size() > common.FileSizeLimit {
		return fmt.Errorf("file exceeds 5GB limit")
	}

	respObj, err := GeneratePresignedURL(ctx, g3, req.Filename, req.FileMetadata, req.Bucket)
	if err != nil {
		return err
	}

	// Generate request with progress bar
	var p *mpb.Progress
	if showProgress {
		p = mpb.New(mpb.WithOutput(os.Stdout))
	}

	fur, err := generateUploadRequest(ctx, g3, common.FileUploadRequestObject{
		FilePath:     req.FilePath,
		Filename:     req.Filename,
		PresignedURL: respObj.URL,
		GUID:         respObj.GUID,
		Bucket:       req.Bucket,
	}, file, p)
	if err != nil {
		return err
	}

	return MultipartUpload(ctx, g3, fur, showProgress)
}
