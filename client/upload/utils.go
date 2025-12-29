package upload

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
)

func SeparateSingleAndMultipartUploads(g3i client.Gen3Interface, objects []common.FileUploadRequestObject) ([]common.FileUploadRequestObject, []common.FileUploadRequestObject) {
	fileSizeLimit := common.FileSizeLimit

	var singlepartObjects []common.FileUploadRequestObject
	var multipartObjects []common.FileUploadRequestObject

	for _, object := range objects {
		fi, err := os.Stat(object.FilePath)
		if err != nil {
			if os.IsNotExist(err) {
				g3i.Logger().Printf("The file you specified \"%s\" does not exist locally\n", object.FilePath)
			} else {
				g3i.Logger().Println("File stat error: " + err.Error())
			}
			g3i.Logger().Failed(object.FilePath, object.Filename, object.FileMetadata, object.GUID, 0, false)
			continue
		}
		if fi.IsDir() {
			continue
		}
		if _, ok := g3i.Logger().GetSucceededLogMap()[object.FilePath]; ok {
			g3i.Logger().Println("File \"" + object.FilePath + "\" found in history. Skipping.")
			continue
		}
		if fi.Size() > common.MultipartFileSizeLimit {
			g3i.Logger().Printf("File %s exceeds max limit\n", fi.Name())
			continue
		}
		if fi.Size() > int64(fileSizeLimit) {
			multipartObjects = append(multipartObjects, object)
		} else {
			singlepartObjects = append(singlepartObjects, object)
		}
	}
	return singlepartObjects, multipartObjects
}

// ProcessFilename returns an FileInfo object which has the information about the path and name to be used for upload of a file
func ProcessFilename(logger logs.Logger, uploadPath string, filePath string, objectId string, includeSubDirName bool, includeMetadata bool) (common.FileUploadRequestObject, error) {
	var err error
	filePath, err = common.GetAbsolutePath(filePath)
	if err != nil {
		return common.FileUploadRequestObject{}, err
	}

	filename := filepath.Base(filePath) // Default to base filename

	var metadata common.FileMetadata
	if includeSubDirName {
		absUploadPath, err := common.GetAbsolutePath(uploadPath)
		if err != nil {
			return common.FileUploadRequestObject{}, err
		}

		// Ensure absUploadPath is a directory path for relative calculation
		// Trim the optional wildcard if present
		uploadDir := strings.TrimSuffix(absUploadPath, common.PathSeparator+"*")
		fileInfo, err := os.Stat(uploadDir)
		if err != nil {
			return common.FileUploadRequestObject{}, err
		}
		if fileInfo.IsDir() {
			// Calculate the path of the file relative to the upload directory
			relPath, err := filepath.Rel(uploadDir, filePath)
			if err != nil {
				return common.FileUploadRequestObject{}, err
			}
			filename = relPath
		}
	}

	if includeMetadata {
		// The metadata path is the file name plus '_metadata.json'
		metadataFilePath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + "_metadata.json"
		var metadataFileBytes []byte
		if _, err := os.Stat(metadataFilePath); err == nil {
			metadataFileBytes, err = os.ReadFile(metadataFilePath)
			if err != nil {
				return common.FileUploadRequestObject{}, errors.New("Error reading metadata file " + metadataFilePath + ": " + err.Error())
			}
			err := json.Unmarshal(metadataFileBytes, &metadata)
			if err != nil {
				return common.FileUploadRequestObject{}, errors.New("Error parsing metadata file " + metadataFilePath + ": " + err.Error())
			}
		} else {
			// No metadata file was found for this file -- proceed, but warn the user.
			logger.Printf("WARNING: File metadata is enabled, but could not find the metadata file %v for file %v. Execute `data-client upload --help` for more info on file metadata.\n", metadataFilePath, filePath)
		}
	}
	return common.FileUploadRequestObject{FilePath: filePath, Filename: filename, FileMetadata: metadata, GUID: objectId}, nil
}

// FormatSize helps to parse a int64 size into string
func FormatSize(size int64) string {
	var unitSize int64
	switch {
	case size >= common.TB:
		unitSize = common.TB
	case size >= common.GB:
		unitSize = common.GB
	case size >= common.MB:
		unitSize = common.MB
	case size >= common.KB:
		unitSize = common.KB
	default:
		unitSize = common.B
	}

	var unitMap = map[int64]string{
		common.B:  "B",
		common.KB: "KB",
		common.MB: "MB",
		common.GB: "GB",
		common.TB: "TB",
	}

	return fmt.Sprintf("%.1f"+unitMap[unitSize], float64(size)/float64(unitSize))
}
