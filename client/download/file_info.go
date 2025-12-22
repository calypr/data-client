package download

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	client "github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
	req "github.com/calypr/data-client/client/request"
)

// AskGen3ForFileInfo resolves filename and size from Shepherd or Indexd
func AskGen3ForFileInfo(
	g3i client.Gen3Interface,
	guid, protocol, downloadPath, filenameFormat string,
	rename bool,
	renamedFiles *[]RenamedOrSkippedFileInfo,
) (string, int64) {
	filename, size, err := resolveFileInfo(g3i, guid, protocol, downloadPath, filenameFormat, rename, renamedFiles)
	if err != nil {
		// On error we already logged and fell back inside resolveFileInfo
		return filename, size
	}
	return filename, size
}

func resolveFileInfo(
	g3i client.Gen3Interface,
	guid, protocol, downloadPath, filenameFormat string,
	rename bool,
	renamedFiles *[]RenamedOrSkippedFileInfo,
) (string, int64, error) {
	hasShepherd, err := g3i.CheckForShepherdAPI(g3i.GetCredential())
	if err != nil {
		g3i.Logger().Println("Error checking Shepherd API: " + err.Error())
		g3i.Logger().Println("Falling back to Indexd...")
		hasShepherd = false
	}

	if hasShepherd {
		return fetchFromShepherd(g3i, guid, downloadPath, filenameFormat, renamedFiles)
	}
	return fetchFromIndexd(g3i, guid, protocol, downloadPath, filenameFormat, rename, renamedFiles)
}

func fetchFromShepherd(
	g3i client.Gen3Interface,
	guid, downloadPath, filenameFormat string,
	renamedFiles *[]RenamedOrSkippedFileInfo,
) (string, int64, error) {
	endpoint := common.ShepherdEndpoint + "/objects/" + guid
	res, err := g3i.DoAuthenticatedRequest(g3i.GetCredential(), &req.RequestBuilder{
		Url:    endpoint,
		Method: http.MethodGet,
	})

	if err != nil {
		return fallbackToGUID(g3i.Logger(), guid, filenameFormat, renamedFiles), 0, err
	}
	defer res.Body.Close()

	var decoded struct {
		Record struct {
			FileName string `json:"file_name"`
			Size     int64  `json:"size"`
		} `json:"record"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return fallbackToGUID(g3i.Logger(), guid, filenameFormat, renamedFiles), 0, err
	}

	return applyFilenameFormat(decoded.Record.FileName, guid, downloadPath, filenameFormat, false, renamedFiles), decoded.Record.Size, nil
}

func fetchFromIndexd(
	g3i client.Gen3Interface,
	guid, protocol, downloadPath, filenameFormat string,
	rename bool,
	renamedFiles *[]RenamedOrSkippedFileInfo,
) (string, int64, error) {

	resp, err := g3i.DoAuthenticatedRequest(
		g3i.GetCredential(),
		&req.RequestBuilder{
			Url:    common.IndexdIndexEndpoint + "/" + guid,
			Method: http.MethodPost,
		},
	)
	if err != nil {

		return fallbackToGUID(g3i.Logger(), guid, filenameFormat, renamedFiles), 0, fmt.Errorf("Error in fetch FromIndexd: %s", err)
	}
	defer resp.Body.Close()

	msg, err := g3i.ParseFenceURLResponse(resp)
	if err != nil {
		return fallbackToGUID(g3i.Logger(), guid, filenameFormat, renamedFiles), 0, err
	}

	if filenameFormat == "guid" {
		return guid, msg.Size, nil
	}

	baseName := msg.FileName
	if baseName == "" {
		baseName = guessFilenameFromURLs(msg.URLs, protocol, g3i.Logger(), guid, renamedFiles)
	}

	return applyFilenameFormat(baseName, guid, downloadPath, filenameFormat, rename, renamedFiles), msg.Size, nil
}

func guessFilenameFromURLs(urls []string, protocol string, logger logs.Logger, guid string, renamedFiles *[]RenamedOrSkippedFileInfo) string {
	if len(urls) == 0 {
		logger.Println("No filename or URLs in Indexd record for " + guid)
		logger.Println("Download likely to fail — check Indexd!")
		return fallbackToGUID(logger, guid, "original", renamedFiles)
	}

	url := urls[0]
	if protocol != "" {
		for _, u := range urls {
			if strings.HasPrefix(u, protocol) {
				url = u
				break
			}
		}
	}

	parts := strings.Split(url, "/")
	name := parts[len(parts)-1]
	if name == "" {
		logger.Println("Failed to guess filename for " + guid)
		return fallbackToGUID(logger, guid, "original", renamedFiles)
	}
	return name
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

func fallbackToGUID(logger logs.Logger, guid, format string, renamedFiles *[]RenamedOrSkippedFileInfo) string {
	logger.Println("Using GUID as filename")
	if format != "guid" {
		*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{
			GUID:        guid,
			OldFilename: "N/A",
			NewFilename: guid,
		})
	}
	return guid
}
