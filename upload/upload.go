package upload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/calypr/data-client/common"
	client "github.com/calypr/data-client/g3client"
	drs "github.com/calypr/data-client/indexd/drs" // Imported for DRSObject
	"github.com/vbauerster/mpb/v8"
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
		return UploadSingle(ctx, g3.GetCredential().Profile, req.GUID, req.OID, req.FilePath, req.Bucket, true, req.Progress)
	}
	g3.Logger().Printf("File size %d bytes (>= 5GB), performing multipart upload\n", fileSize)
	return MultipartUpload(ctx, g3, req, file, showProgress)
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
		Progress:     req.Progress,
		OID:          req.OID,
	}, file, p)
	if err != nil {
		return err
	}

	return MultipartUpload(ctx, g3, fur, file, showProgress)
}

// RegisterAndUploadFile orchestrates registration with Indexd and uploading via Fence.
// It handles checking for existing records, upsert logic, checking if file is already downloadable, and performing the upload.
func RegisterAndUploadFile(ctx context.Context, g3 client.Gen3Interface, drsObject *drs.DRSObject, filePath string, bucketName string, upsert bool) (*drs.DRSObject, error) {
	// 1. Register with Indexd
	// Note: The caller is responsible for converting local DRS object to data-client DRS object if needed.

	res, err := g3.Indexd().RegisterRecord(ctx, drsObject)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			if !upsert {
				g3.Logger().Printf("indexd record already exists, proceeding for %s\n", drsObject.Id)
			} else {
				g3.Logger().Printf("indexd record already exists, deleting and re-adding for %s\n", drsObject.Id)
				err = g3.Indexd().DeleteIndexdRecord(ctx, drsObject.Id)
				if err != nil {
					return nil, fmt.Errorf("failed to delete existing record: %w", err)
				}
				res, err = g3.Indexd().RegisterRecord(ctx, drsObject)
				if err != nil {
					return nil, fmt.Errorf("failed to re-register record: %w", err)
				}
			}
		} else {
			return nil, fmt.Errorf("error registering indexd record: %w", err)
		}
	} else {
		// If registration succeeded, use the returned object which might have updated fields (e.g. created time)
		// although we typically reuse the ID for upload.
	}

	// If we didn't get a new object (upsert=false case), we should fetch the existing one to be sure about its state?
	// But we have the ID in drsObject.Id.

	// 2. Check if file is downloadable
	downloadable, err := isFileDownloadable(ctx, g3, drsObject.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to check if file is downloadable: %w", err)
	}

	if downloadable {
		g3.Logger().Printf("File %s is already downloadable, skipping upload.\n", drsObject.Id)
		// Return the registered object (or the one passed in if we didn't re-register)
		// If we re-registered, res is populated. If not, we might want to return the fetched object?
		// For consistency, let's return res if set, or fetch it.
		if res != nil {
			return res, nil
		}
		return g3.Indexd().GetObject(ctx, drsObject.Id)
	}

	// 3. Upload File
	req := common.FileUploadRequestObject{
		FilePath: filePath,
		Filename: filepath.Base(filePath),
		GUID:     drsObject.Id,
		OID:      drsObject.Id,
		Bucket:   bucketName,
	}

	// Use Upload function which handles single/multipart selection
	err = Upload(ctx, g3, req, false)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	// Return the object
	if res != nil {
		return res, nil
	}
	return g3.Indexd().GetObject(ctx, drsObject.Id)
}

func isFileDownloadable(ctx context.Context, g3 client.Gen3Interface, did string) (bool, error) {
	// Get the object to find access methods
	obj, err := g3.Indexd().GetObject(ctx, did)
	if err != nil {
		return false, err
	}

	if len(obj.AccessMethods) == 0 {
		return false, nil
	}

	accessType := obj.AccessMethods[0].Type
	res, err := g3.Indexd().GetDownloadURL(ctx, did, accessType)
	if err != nil {
		// If we can't get a download URL, it's not downloadable
		return false, nil
	}

	if res.URL == "" {
		return false, nil
	}

	// Check if the URL is accessible
	err = common.CanDownloadFile(res.URL)
	return err == nil, nil
}
