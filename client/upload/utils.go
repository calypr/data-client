package upload

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/calypr/data-client/client/logs"
	req "github.com/calypr/data-client/client/request"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// InitMultipartUpload helps sending requests to FENCE to init a multipart upload
func initMultipartUpload(g3 client.Gen3Interface, furObject common.FileUploadRequestObject, bucketName string) (string, string, error) {
	// Use Filename and GUID directly from the unified request object
	multipartInitObject := InitRequestObject{Filename: furObject.Filename, Bucket: bucketName, GUID: furObject.GUID}

	objectBytes, err := json.Marshal(multipartInitObject)
	if err != nil {
		return "", "", errors.New("Error has occurred during marshalling data for multipart upload initialization, detailed error message: " + err.Error())
	}

	resp, err := g3.DoAuthenticatedRequest(
		g3.GetCredential(),
		&req.RequestBuilder{
			Method:  http.MethodPost,
			Url:     common.FenceDataMultipartInitEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Body:    objectBytes,
		},
	)

	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return "", "", errors.New(err.Error() + "\nPlease check to ensure FENCE version is at 2.8.0 or beyond")
		}
		return "", "", errors.New("Error has occurred during multipart upload initialization, detailed error message: " + err.Error())
	}

	msg, err := g3.ParseFenceURLResponse(resp)
	if err != nil {
		return "", "", errors.New("Error has occurred during multipart upload initialization, detailed error message: " + err.Error())

	}

	if msg.UploadID == "" || msg.GUID == "" {
		return "", "", errors.New("unknown error has occurred during multipart upload initialization. Please check logs from Gen3 services")
	}
	return msg.UploadID, msg.GUID, err
}

// GenerateMultipartPresignedURL helps sending requests to FENCE to get a presigned URL for a part during a multipart upload
func generateMultipartPresignedURL(g3 client.Gen3Interface, key string, uploadID string, partNumber int, bucketName string) (string, error) {
	multipartUploadObject := MultipartUploadRequestObject{Key: key, UploadID: uploadID, PartNumber: partNumber, Bucket: bucketName}
	objectBytes, err := json.Marshal(multipartUploadObject)
	if err != nil {
		return "", errors.New("Error has occurred during marshalling data for multipart upload presigned url generation, detailed error message: " + err.Error())
	}

	resp, err := g3.DoAuthenticatedRequest(
		g3.GetCredential(),
		&req.RequestBuilder{
			Url:     common.FenceDataMultipartUploadEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Method:  http.MethodPost,
			Body:    objectBytes,
		},
	)
	if err != nil {
		return "", errors.New("Error has occurred during multipart upload presigned url generation, detailed error message: " + err.Error())
	}

	msg, err := g3.ParseFenceURLResponse(resp)
	if err != nil {
		return "", errors.New("Error has occurred during multipart upload initialization, detailed error message: " + err.Error())
	}

	if msg.PresignedURL == "" {
		return "", errors.New("unknown error has occurred during multipart upload presigned url generation. Please check logs from Gen3 services")
	}
	return msg.PresignedURL, err
}

// CompleteMultipartUpload helps sending requests to FENCE to complete a multipart upload
func CompleteMultipartUpload(g3 client.Gen3Interface, key string, uploadID string, parts []MultipartPartObject, bucketName string) error {
	multipartCompleteObject := MultipartCompleteRequestObject{Key: key, UploadID: uploadID, Parts: parts, Bucket: bucketName}
	objectBytes, err := json.Marshal(multipartCompleteObject)
	if err != nil {
		return errors.New("Error has occurred during marshalling data for multipart upload, detailed error message: " + err.Error())
	}

	// TOOD: error check this, return resp information
	_, err = g3.DoAuthenticatedRequest(
		g3.GetCredential(),
		&req.RequestBuilder{
			Url:     common.FenceDataMultipartCompleteEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Body:    objectBytes,
			Method:  http.MethodPost,
		},
	)
	if err != nil {
		return errors.New("Error has occurred during completing multipart upload, detailed error message: " + err.Error())
	}
	return nil
}

// GenerateUploadRequest helps preparing the HTTP request for upload and the progress bar for single part upload
func generateUploadRequest(g3 client.Gen3Interface, furObject common.FileUploadRequestObject, file *os.File, progress *mpb.Progress) (common.FileUploadRequestObject, error) {
	if furObject.PresignedURL == "" {
		endPointPostfix := common.FenceDataUploadEndpoint + "/" + furObject.GUID + "?file_name=" + url.QueryEscape(furObject.Filename)

		if furObject.Bucket != "" {
			endPointPostfix += "&bucket=" + furObject.Bucket
		}
		resp, err := g3.DoAuthenticatedRequest(
			g3.GetCredential(),
			&req.RequestBuilder{
				Url:     endPointPostfix,
				Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			},
		)

		msg, err := g3.ParseFenceURLResponse(resp)
		if err != nil && !strings.Contains(err.Error(), "No GUID found") {
			return furObject, errors.New("Upload error: " + err.Error())
		}
		if msg.URL == "" {
			return furObject, errors.New("Upload error: error in generating presigned URL for " + furObject.Filename)
		}
		furObject.PresignedURL = msg.URL
	}

	fi, err := file.Stat()
	if err != nil {
		return furObject, errors.New("File stat error for file" + furObject.Filename + ", file may be missing or unreadable because of permissions.\n")
	}

	if fi.Size() > common.FileSizeLimit {
		return furObject, errors.New("The file size of file " + furObject.Filename + " exceeds the limit allowed and cannot be uploaded. The maximum allowed file size is " + FormatSize(common.FileSizeLimit) + ".\n")
	}

	if progress == nil {
		progress = mpb.New(mpb.WithOutput(os.Stdout))
	}
	bar := progress.AddBar(fi.Size(),
		mpb.PrependDecorators(
			decor.Name(furObject.Filename+" "),
			decor.CountersKibiByte("% .1f / % .1f"),
		),
		mpb.AppendDecorators(
			decor.Percentage(),
			decor.AverageSpeed(decor.SizeB1024(0), " % .1f"),
		),
	)
	pr, pw := io.Pipe()

	go func() {
		var writer io.Writer
		defer pw.Close()
		defer file.Close()

		writer = bar.ProxyWriter(pw)
		if _, err = io.Copy(writer, file); err != nil {
			err = errors.New("io.Copy error: " + err.Error() + "\n")
		}
		if err = pw.Close(); err != nil {
			err = errors.New("Pipe writer close error: " + err.Error() + "\n")
		}
	}()
	if err != nil {
		return furObject, err
	}

	req, err := http.NewRequest(http.MethodPut, furObject.PresignedURL, pr)
	req.ContentLength = fi.Size()

	furObject.Request = req
	furObject.Progress = progress
	furObject.Bar = bar

	return furObject, err
}

func SeparateSingleAndMultipartUploads(g3i client.Gen3Interface, objects []common.FileUploadRequestObject, forceMultipart bool) ([]common.FileUploadRequestObject, []common.FileUploadRequestObject) {
	fileSizeLimit := common.FileSizeLimit
	if forceMultipart {
		fileSizeLimit = common.MinMultipartChunkSize
	}

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
