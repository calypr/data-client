package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-multierror"
)

func ToJSONReader(payload any) (io.Reader, error) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode JSON payload: %w", err)
	}
	return &buf, nil
}

// ParseRootPath parses dirname that has "~" in the beginning
func ParseRootPath(filePath string) (string, error) {
	if filePath != "" && filePath[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return homeDir + filePath[1:], nil
	}
	return filePath, nil
}

// GetAbsolutePath parses input file path to its absolute path and removes the "~" in the beginning
func GetAbsolutePath(filePath string) (string, error) {
	fullFilePath, err := ParseRootPath(filePath)
	if err != nil {
		return "", err
	}
	fullFilePath, err = filepath.Abs(fullFilePath)
	return fullFilePath, err
}

// ParseFilePaths generates all possible file paths
func ParseFilePaths(filePath string, metadataEnabled bool) ([]string, error) {
	fullFilePath, err := GetAbsolutePath(filePath)
	if err != nil {
		return []string{}, err
	}
	initialPaths, err := filepath.Glob(fullFilePath)
	if err != nil {
		return []string{}, err
	}

	var multiErr *multierror.Error
	var finalFilePaths []string
	for _, p := range cleanupHiddenFiles(initialPaths) {
		file, err := os.Open(p)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("file open error for %s: %w", p, err))
			continue
		}

		func(filePath string, file *os.File) {
			defer file.Close()

			fi, _ := file.Stat()
			if fi.IsDir() {
				err = filepath.Walk(filePath, func(path string, fileInfo os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					isHidden, err := IsHidden(path)
					if err != nil {
						return err
					}
					isMetadata := false
					if metadataEnabled {
						isMetadata = strings.HasSuffix(path, "_metadata.json")
					}
					if !fileInfo.IsDir() && !isHidden && !isMetadata {
						finalFilePaths = append(finalFilePaths, path)
					}
					return nil
				})
				if err != nil {
					multiErr = multierror.Append(multiErr, fmt.Errorf("directory walk error for %s: %w", filePath, err))
				}
			} else {
				finalFilePaths = append(finalFilePaths, filePath)
			}
		}(p, file)
	}

	return finalFilePaths, multiErr.ErrorOrNil()
}

func cleanupHiddenFiles(filePaths []string) []string {
	i := 0
	for _, filePath := range filePaths {
		isHidden, err := IsHidden(filePath)
		if err != nil {
			log.Println("Error occurred when checking hidden files: " + err.Error())
			continue
		}

		if isHidden {
			log.Printf("File %s is a hidden file and will be skipped\n", filePath)
			continue
		}
		filePaths[i] = filePath
		i++
	}
	return filePaths[:i]
}

// CanDownloadFile checks if a file can be downloaded from the given signed URL
// by issuing a ranged GET for a single byte to mimic HEAD behavior.
func CanDownloadFile(signedURL string) error {
	req, err := http.NewRequest("GET", signedURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error while sending the request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPartialContent || resp.StatusCode == http.StatusOK {
		return nil
	}

	return fmt.Errorf("failed to access file, HTTP status: %d", resp.StatusCode)
}
