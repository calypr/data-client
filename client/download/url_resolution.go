package download

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/request"
)

// GetDownloadResponse gets presigned URL and prepares HTTP response
func GetDownloadResponse(ctx context.Context, g3 client.Gen3Interface, fdr *common.FileDownloadResponseObject, protocolText string) error {
	url, err := g3.GetPresignedUrl(ctx, fdr.GUID, protocolText)
	if err != nil {
		return err
	}
	fdr.URL = url

	if fdr.Range > 0 && !isCloudPresignedURL(url) {
		if !supportsRange(url) {
			fdr.Range = 0
		}
	}

	return makeDownloadRequest(ctx, g3, fdr)
}

func isCloudPresignedURL(url string) bool {
	return strings.Contains(url, "X-Amz-Signature") || strings.Contains(url, "X-Goog-Signature")
}

func supportsRange(url string) bool {
	resp, err := http.Head(url)
	if err != nil || resp.Header.Get("Accept-Ranges") != "bytes" {
		return false
	}
	return true
}

func makeDownloadRequest(ctx context.Context, g3 client.Gen3Interface, fdr *common.FileDownloadResponseObject) error {
	headers := map[string]string{}
	if fdr.Range > 0 {
		headers["Range"] = "bytes=" + strconv.FormatInt(fdr.Range, 10) + "-"
	}

	resp, err := g3.Do(
		ctx,
		&request.RequestBuilder{
			Method:  http.MethodGet,
			Url:     fdr.URL,
			Headers: headers,
		},
	)
	if err != nil {
		return errors.New("Request failed: " + strings.ReplaceAll(err.Error(), fdr.URL, "<SENSITIVE_URL>"))
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return errors.New("Non-OK response: " + strconv.Itoa(resp.StatusCode))
	}
	fdr.Response = resp
	return nil
}
