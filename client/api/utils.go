package api

import (
	"bytes"
	"net/http"
)

type Message any
type Response any

type FenceResponse struct {
	URL          string   `json:"url"`
	GUID         string   `json:"guid"`
	UploadID     string   `json:"uploadId"`
	PresignedURL string   `json:"presigned_url"`
	FileName     string   `json:"file_name"`
	URLs         []string `json:"urls"`
	Size         int64    `json:"size"`
}

func ResponseToString(resp *http.Response) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body) // nolint: errcheck
	return buf.String()
}
