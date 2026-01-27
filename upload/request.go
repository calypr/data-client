package upload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	client "github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/common"
	req "github.com/calypr/data-client/request"
	"github.com/vbauerster/mpb/v8"
)

// GeneratePresignedURL handles both Shepherd and Fence fallback
func GeneratePresignedUploadURL(ctx context.Context, g3 client.Gen3Interface, filename string, metadata common.FileMetadata, bucket string) (*PresignedURLResponse, error) {
	hasShepherd, err := g3.Fence().CheckForShepherdAPI(ctx)
	if err != nil || !hasShepherd {
		msg, err := g3.Fence().InitUpload(ctx, filename, bucket, "")
		if err != nil {
			return nil, err
		}
		return &PresignedURLResponse{URL: msg.URL, GUID: msg.GUID}, nil
	}

	shepherdPayload := ShepherdInitRequestObject{
		Filename: filename,
		Authz: ShepherdAuthz{
			Version: "0", ResourcePaths: metadata.Authz,
		},
		Aliases:  metadata.Aliases,
		Metadata: metadata.Metadata,
	}

	reader, err := common.ToJSONReader(shepherdPayload)
	if err != nil {
		return nil, err
	}

	cred := g3.GetCredential()
	r, err := g3.Fence().Do(
		ctx,
		&req.RequestBuilder{
			Url:    cred.APIEndpoint + common.ShepherdEndpoint + "/objects",
			Method: http.MethodPost,
			Body:   reader,
			Token:  cred.AccessToken,
		})
	if err != nil || r.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("shepherd upload init failed")
	}

	var res PresignedURLResponse
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

// GenerateUploadRequest helps preparing the HTTP request for upload and the progress bar for single part upload
func generateUploadRequest(ctx context.Context, g3 client.Gen3Interface, furObject common.FileUploadRequestObject, file *os.File, progress *mpb.Progress) (common.FileUploadRequestObject, error) {
	if furObject.PresignedURL == "" {
		msg, err := g3.Fence().GetUploadPresignedUrl(ctx, furObject.GUID, furObject.Filename, furObject.Bucket)
		if err != nil && !strings.Contains(err.Error(), "No GUID found") {
			return furObject, fmt.Errorf("Upload error: %w", err)
		}
		if msg.URL == "" {
			return furObject, errors.New("Upload error: error in generating presigned URL for " + furObject.Filename)
		}
		furObject.PresignedURL = msg.URL
	}

	fi, err := file.Stat()
	if err != nil {
		return furObject, errors.New("File stat error for file" + furObject.Filename + ", file may be missing or unreadable because of permissions.\n")
	}

	if fi.Size() > common.FileSizeLimit {
		return furObject, errors.New("The file size of file " + furObject.Filename + " exceeds the limit allowed and cannot be uploaded. The maximum allowed file size is " + FormatSize(common.FileSizeLimit) + ".\n")
	}

	return furObject, err
}
