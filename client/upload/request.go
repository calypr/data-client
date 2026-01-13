package upload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	client "github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	req "github.com/calypr/data-client/client/request"
	"github.com/vbauerster/mpb/v8"
)

// GeneratePresignedURL handles both Shepherd and Fence fallback
func GeneratePresignedUploadURL(ctx context.Context, g3 client.Gen3Interface, filename string, metadata common.FileMetadata, bucket string) (*PresignedURLResponse, error) {
	hasShepherd, err := g3.CheckForShepherdAPI(ctx)
	if err != nil || !hasShepherd {
		payload := map[string]string{
			"file_name": filename,
		}
		if bucket != "" {
			payload["bucket"] = bucket
		}

		buf, err := common.ToJSONReader(payload)
		if err != nil {
			return nil, err
		}

		cred := g3.GetCredential()
		resp, err := g3.Do(
			ctx,
			&req.RequestBuilder{
				Method:  http.MethodPost,
				Url:     cred.APIEndpoint + common.FenceDataUploadEndpoint,
				Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
				Body:    buf,
				Token:   cred.AccessToken,
			})
		if err != nil {
			return nil, err
		}
		msg, err := g3.ParseFenceURLResponse(resp)
		return &PresignedURLResponse{msg.URL, msg.GUID}, err
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
	r, err := g3.Do(
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
		endPointPostfix := common.FenceDataUploadEndpoint + "/" + furObject.GUID + "?file_name=" + url.QueryEscape(furObject.Filename)

		if furObject.Bucket != "" {
			endPointPostfix += "&bucket=" + furObject.Bucket
		}
		cred := g3.GetCredential()
		resp, err := g3.Do(
			ctx,
			&req.RequestBuilder{
				Url:     cred.APIEndpoint + endPointPostfix,
				Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
				Token:   cred.AccessToken,
				Method:  http.MethodGet,
			},
		)
		if err != nil {
			return furObject, errors.New("Upload error: " + err.Error())
		}

		msg, err := g3.ParseFenceURLResponse(resp)
		if err != nil && !strings.Contains(err.Error(), "No GUID found") {
			return furObject, errors.New("Upload error: " + err.Error())
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
