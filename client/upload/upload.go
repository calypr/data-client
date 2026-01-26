package upload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/request"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// Upload is a unified catch-all function that automatically chooses between
// single-part and multipart upload based on file size.
func Upload(ctx context.Context, g3 client.Gen3Interface, req common.FileUploadRequestObject, showProgress bool) error {
	g3.Logger().Printf("Processing Upload Request for: %s\n", req.FilePath)

	file, err := os.Open(req.FilePath)
	if err != nil {
		return fmt.Errorf("cannot open file %s: %w", req.FilePath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat file: %w", err)
	}

	fileSize := stat.Size()
	if fileSize == 0 {
		return fmt.Errorf("file is empty: %s", req.Filename)
	}

	// Use Single-Part if file is smaller than 5GB (or your defined limit)
	if fileSize < 5*common.GB {
		g3.Logger().Printf("File size %d bytes (< 5GB), performing single-part upload\n", fileSize)
		UploadSingle(ctx, g3.GetCredential().Profile, req.GUID, req.GUID, req.FilePath, req.Bucket, true, nil)
	}
	g3.Logger().Printf("File size %d bytes (>= 5GB), performing multipart upload\n", fileSize)
	return MultipartUpload(ctx, g3, req, file, showProgress)
}

func performSinglePartUpload(ctx context.Context, g3 client.Gen3Interface, req common.FileUploadRequestObject, showProgress bool) error {
	// 1. Get the Presigned URL
	respObj, err := GeneratePresignedUploadURL(ctx, g3, req.Filename, req.FileMetadata, req.Bucket)
	if err != nil {
		return fmt.Errorf("failed to generate single-part URL: %w", err)
	}

	req.GUID = respObj.GUID
	req.PresignedURL = respObj.URL

	// 2. Open file and setup progress
	file, _ := os.Open(req.FilePath)
	defer file.Close()

	var body io.Reader = file
	var p *mpb.Progress
	if showProgress {
		p = mpb.New(mpb.WithOutput(os.Stdout))
		fi, _ := file.Stat()
		bar := p.AddBar(fi.Size(),
			mpb.PrependDecorators(decor.Name(req.Filename+" ")),
			mpb.AppendDecorators(decor.Percentage()),
		)
		body = bar.ProxyReader(file)
	}

	resp, err := g3.Do(ctx, &request.RequestBuilder{
		Method: http.MethodPut,
		Url:    req.PresignedURL,
		Body:   body,
	})

	if p != nil {
		p.Wait()
	}

	if err != nil || resp.StatusCode != http.StatusOK {
		return fmt.Errorf("single-part upload failed")
	}
	return nil
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

	respObj, err := GeneratePresignedUploadURL(ctx, g3, req.Filename, req.FileMetadata, req.Bucket)
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

	return MultipartUpload(ctx, g3, fur, file, showProgress)
}
