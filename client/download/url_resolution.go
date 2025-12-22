package download

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	req "github.com/calypr/data-client/client/request"
)

// GetDownloadResponse gets presigned URL and prepares HTTP response
func GetDownloadResponse(g3 client.Gen3Interface, fdr *common.FileDownloadResponseObject, protocolText string) error {
	url, err := resolvePresignedURL(g3, fdr.GUID, protocolText)
	if err != nil {
		return err
	}
	fdr.URL = url

	if fdr.Range > 0 && !isCloudPresignedURL(url) {
		if !supportsRange(url) {
			fdr.Range = 0
		}
	}

	return makeDownloadRequest(g3, fdr)
}

func resolvePresignedURL(g3 client.Gen3Interface, guid, protocolText string) (string, error) {
	hasShepherd, _ := g3.CheckForShepherdAPI(g3.GetCredential()) // error already logged upstream
	if hasShepherd {
		return resolveFromShepherd(g3, guid)
	}
	return resolveFromFence(g3, guid, protocolText)
}

func resolveFromShepherd(g3 client.Gen3Interface, guid string) (string, error) {
	endpoint := common.ShepherdEndpoint + "/objects/" + guid + "/download"
	r, err := g3.DoAuthenticatedRequest(
		g3.GetCredential(),
		&req.RequestBuilder{
			Url:    endpoint,
			Method: http.MethodGet,
		},
	)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()

	if r.StatusCode != 200 {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		return "", errors.New("Shepherd non-200: " + strconv.Itoa(r.StatusCode) + " — " + buf.String())
	}

	var resp struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil || resp.URL == "" {
		return "", errors.New("Failed to parse Shepherd download URL")
	}
	return resp.URL, nil
}

func resolveFromFence(g3 client.Gen3Interface, guid, protocolText string) (string, error) {
	endpoint := common.FenceDataDownloadEndpoint + "/" + guid + protocolText

	resp, err := g3.DoAuthenticatedRequest(
		g3.GetCredential(),
		&req.RequestBuilder{
			Url:    endpoint,
			Method: http.MethodGet,
		},
	)
	if err != nil {
		return "", errors.New("Failed to get URL from Fence via DoAuthenticatedRequest: " + err.Error())
	}
	defer resp.Body.Close()

	msg, err := g3.ParseFenceURLResponse(resp)
	if err != nil || msg.URL == "" {
		return "", errors.New("Failed to get URL from Fence via ParseFenceURLResponse: " + err.Error())
	}
	return msg.URL, nil
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

func makeDownloadRequest(g3 client.Gen3Interface, fdr *common.FileDownloadResponseObject) error {
	headers := map[string]string{}
	if fdr.Range > 0 {
		headers["Range"] = "bytes=" + strconv.FormatInt(fdr.Range, 10) + "-"
	}

	resp, err := g3.DoAuthenticatedRequest(
		g3.GetCredential(),
		&req.RequestBuilder{
			Method:  http.MethodGet,
			Url:     fdr.URL,
			Headers: headers,
			Timeout: true,
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
