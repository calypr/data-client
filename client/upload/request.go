package upload

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/gen3Client"
	req "github.com/calypr/data-client/client/request"
)

// GeneratePresignedURL handles both Shepherd and Fence fallback
func generatePresignedURL(g3 gen3Client.Gen3Interface, filename string, metadata common.FileMetadata, bucket string) (string, string, error) {
	hasShepherd, err := g3.CheckForShepherdAPI(g3.GetCredential())
	if err != nil || !hasShepherd {
		// Fallback to Fence
		payload := map[string]string{
			"file_name": filename,
		}
		if bucket != "" {
			payload["bucket"] = bucket
		}
		body, _ := json.Marshal(payload)

		resp, err := g3.DoAuthenticatedRequest(g3.GetCredential(), &req.RequestBuilder{
			Method:  http.MethodPost,
			Url:     common.FenceDataUploadEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Body:    body,
		})
		if err != nil {
			return "", "", err
		}
		msg, err := g3.ParseFenceURLResponse(resp)
		return msg.URL, msg.GUID, err
	}

	// Shepherd path
	shepherdPayload := struct {
		Filename string `json:"file_name"`
		Authz    struct {
			Version       string
			ResourcePaths []string
		} `json:"authz"`
		Aliases  []string       `json:"aliases"`
		Metadata map[string]any `json:"metadata"`
	}{
		Filename: filename,
		Authz: struct {
			Version       string
			ResourcePaths []string
		}{Version: "0", ResourcePaths: metadata.Authz},
		Aliases:  metadata.Aliases,
		Metadata: metadata.Metadata,
	}
	body, _ := json.Marshal(shepherdPayload)

	r, err := g3.DoAuthenticatedRequest(g3.GetCredential(), &req.RequestBuilder{
		Url:    common.ShepherdEndpoint + "/objects",
		Method: http.MethodPost,
		Body:   body,
	})
	if err != nil || r.StatusCode != http.StatusCreated {
		return "", "", errors.New("shepherd upload init failed")
	}

	var res struct {
		GUID string `json:"guid"`
		URL  string `json:"upload_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		return "", "", err
	}
	return res.URL, res.GUID, nil
}
