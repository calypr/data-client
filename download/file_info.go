package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	client "github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/request"
)

func AskGen3ForFileInfo(
	ctx context.Context,
	g3i client.Gen3Interface,
	guid, protocol, downloadPath, filenameFormat string,
	rename bool,
	renamedFiles *[]RenamedOrSkippedFileInfo,
) (*IndexdResponse, error) {
	hasShepherd, err := g3i.Fence().CheckForShepherdAPI(ctx)
	if err != nil {
		g3i.Logger().Println("Error checking Shepherd API: " + err.Error())
		g3i.Logger().Println("Falling back to Indexd...")
		hasShepherd = false
	}

	if hasShepherd {
		info, err := fetchFromShepherd(ctx, g3i, guid, downloadPath, filenameFormat, renamedFiles)
		if err == nil {
			return info, nil
		}
		g3i.Logger().Printf("Shepherd fetch failed for %s: %v. Falling back to Indexd...\n", guid, err)
	}
	info, err := fetchFromIndexd(ctx, g3i, http.MethodGet, guid, protocol, downloadPath, filenameFormat, rename, renamedFiles)
	if err != nil {
		g3i.Logger().Printf("All meta-data lookups failed for %s: %v. Using GUID as default filename.\n", guid, err)
		*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{GUID: guid, OldFilename: guid, NewFilename: guid})
		return &IndexdResponse{guid, 0}, nil
	}
	return info, nil
}

func fetchFromShepherd(
	ctx context.Context,
	g3i client.Gen3Interface,
	guid, downloadPath, filenameFormat string,
	renamedFiles *[]RenamedOrSkippedFileInfo,
) (*IndexdResponse, error) {
	cred := g3i.GetCredential()
	res, err := g3i.Fence().Do(ctx,
		&request.RequestBuilder{
			Url:    cred.APIEndpoint + "/" + cred.AccessToken + common.ShepherdEndpoint + "/objects/" + guid,
			Method: http.MethodGet,
			Token:  cred.AccessToken,
		})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var decoded struct {
		Record struct {
			FileName string `json:"file_name"`
			Size     int64  `json:"size"`
		} `json:"record"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return nil, err
	}

	return &IndexdResponse{applyFilenameFormat(decoded.Record.FileName, guid, downloadPath, filenameFormat, false, renamedFiles), decoded.Record.Size}, nil
}

func fetchFromIndexd(
	ctx context.Context,
	g3i client.Gen3Interface, method,
	guid, protocol, downloadPath, filenameFormat string,
	rename bool,
	renamedFiles *[]RenamedOrSkippedFileInfo,
) (*IndexdResponse, error) {

	cred := g3i.GetCredential()
	resp, err := g3i.Fence().Do(
		ctx,
		&request.RequestBuilder{
			Url:    cred.APIEndpoint + common.IndexdIndexEndpoint + "/" + guid,
			Method: method,
			Token:  cred.AccessToken,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error in fetch FromIndexd: %s", err)
	}

	defer resp.Body.Close()
	msg, err := g3i.Fence().ParseFenceURLResponse(resp)
	if err != nil {
		return nil, err
	}

	if filenameFormat == "guid" {
		return &IndexdResponse{guid, msg.Size}, nil
	}

	if msg.FileName == "" {
		return nil, fmt.Errorf("FileName is a required field in Indexd to download the file, but upload record %#v does not contain it", msg)
	}

	return &IndexdResponse{applyFilenameFormat(msg.FileName, guid, downloadPath, filenameFormat, rename, renamedFiles), msg.Size}, nil
}

func applyFilenameFormat(baseName, guid, downloadPath, format string, rename bool, renamedFiles *[]RenamedOrSkippedFileInfo) string {
	switch format {
	case "guid":
		return guid
	case "combined":
		return guid + "_" + baseName
	case "original":
		if !rename {
			return baseName
		}
		newName := processOriginalFilename(downloadPath, baseName)
		if newName != baseName {
			*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{
				GUID:        guid,
				OldFilename: baseName,
				NewFilename: newName,
			})
		}
		return newName
	default:
		return baseName
	}
}
