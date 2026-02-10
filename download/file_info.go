package download

import (
	"context"

	"github.com/calypr/data-client/backend"
)

func GetFileInfo(
	ctx context.Context,
	bk backend.Backend,
	guid, protocol, downloadPath, filenameFormat string,
	rename bool,
	renamedFiles *[]RenamedOrSkippedFileInfo,
) (*IndexdResponse, error) {
	// Use Backend to get object details
	drsObj, err := bk.GetFileDetails(ctx, guid)
	if err != nil {
		bk.Logger().Warn("Failed to get file details", "guid", guid, "error", err)
		// Fallback: use GUID as filename if failed?
		// Original code: "All meta-data lookups failed... Using GUID as default"
		*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{GUID: guid, OldFilename: guid, NewFilename: guid})
		return &IndexdResponse{Name: guid, Size: 0}, nil
	}

	name := drsObj.Name
	if name == "" {
		// If name is empty (some DRS servers might not return it?), use GUID
		name = guid
	}

	finalName := applyFilenameFormat(name, guid, downloadPath, filenameFormat, rename, renamedFiles)
	return &IndexdResponse{Name: finalName, Size: drsObj.Size}, nil
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
