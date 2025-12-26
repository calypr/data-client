package upload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	client "github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	req "github.com/calypr/data-client/client/request"
)

type PresignedURLResponse struct {
	GUID string `json:"guid"`
	URL  string `json:"upload_url"`
}

// GeneratePresignedURL handles both Shepherd and Fence fallback
func GeneratePresignedURL(ctx context.Context, g3 client.Gen3Interface, filename string, metadata common.FileMetadata, bucket string) (*PresignedURLResponse, error) {
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
