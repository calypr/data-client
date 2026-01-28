package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/calypr/data-client/common"
	client "github.com/calypr/data-client/g3client"
)

// GetDownloadResponse gets presigned URL and prepares HTTP response
func GetDownloadResponse(ctx context.Context, g3 client.Gen3Interface, fdr *common.FileDownloadResponseObject, protocolText string) error {
	// 1. Try Fence first
	url, err := g3.Fence().GetDownloadPresignedUrl(ctx, fdr.GUID, protocolText)
	if err == nil && url != "" {
		fdr.PresignedURL = url
	} else {
		// 2. Fallback to IndexD DRS endpoint
		accessType := "s3"
		if strings.HasPrefix(protocolText, "?protocol=") {
			accessType = strings.TrimPrefix(protocolText, "?protocol=")
		} else if protocolText == "?protocol=gs" {
			accessType = "gs"
		}

		accessURL, errIdx := g3.Indexd().GetDownloadURL(ctx, fdr.GUID, accessType)
		if errIdx == nil && accessURL != nil && accessURL.URL != "" {
			fdr.PresignedURL = accessURL.URL
			// Some DRS providers might return required headers
			// This is not currently used by makeDownloadRequest but good to have for future
		} else {
			if err != nil {
				return err
			}
			if errIdx != nil {
				return errIdx
			}
			return fmt.Errorf("failed to resolve download URL for %s", fdr.GUID)
		}
	}

	return makeDownloadRequest(ctx, g3, fdr)
}

func isCloudPresignedURL(url string) bool {
	return strings.Contains(url, "X-Amz-Signature") ||
		strings.Contains(url, "X-Goog-Signature") ||
		strings.Contains(url, "Signature=") ||
		strings.Contains(url, "AWSAccessKeyId=") ||
		strings.Contains(url, "Expires=")
}

func makeDownloadRequest(ctx context.Context, g3 client.Gen3Interface, fdr *common.FileDownloadResponseObject) error {
	skipAuth := isCloudPresignedURL(fdr.PresignedURL)
	rb := g3.Fence().New(http.MethodGet, fdr.PresignedURL).WithSkipAuth(skipAuth)

	if fdr.Range > 0 {
		rb.WithHeader("Range", "bytes="+strconv.FormatInt(fdr.Range, 10)+"-")
	}

	resp, err := g3.Fence().Do(ctx, rb)

	if err != nil {
		return errors.New("Request failed: " + strings.ReplaceAll(err.Error(), fdr.PresignedURL, "<SENSITIVE_URL>"))
	}

	// Check for non-success status codes
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		defer resp.Body.Close() // Ensure the body is closed

		bodyBytes, err := io.ReadAll(resp.Body)
		bodyString := "<unable to read body>"
		if err == nil {
			bodyString = string(bodyBytes)
		}

		return fmt.Errorf("non-OK response: %d, body: %s", resp.StatusCode, bodyString)
	}

	fdr.Response = resp
	return nil
}
