package common

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	// B is bytes
	B int64 = iota
	// KB is kilobytes
	KB int64 = 1 << (10 * iota)
	// MB is megabytes
	MB
	// GB is gigabytes
	GB
	// TB is terabytes
	TB
)
const (
	// DefaultUseShepherd sets whether gen3client will attempt to use the Shepherd / Object Management API
	// endpoints if available.
	// The user can override this default using the `data-client configure` command.
	DefaultUseShepherd = false

	// DefaultMinShepherdVersion is the minimum version of Shepherd that the gen3client will use.
	// Before attempting to use Shepherd, the client will check for Shepherd's version, and if the version is
	// below this number the gen3client will instead warn the user and fall back to fence/indexd.
	// The user can override this default using the `data-client configure` command.
	DefaultMinShepherdVersion = "2.0.0"

	// ShepherdEndpoint is the endpoint postfix for SHEPHERD / the Object Management API
	ShepherdEndpoint = "/mds"

	// ShepherdVersionEndpoint is the endpoint used to check what version of Shepherd a commons has deployed
	ShepherdVersionEndpoint = "/mds/version"

	// IndexdIndexEndpoint is the endpoint postfix for INDEXD index
	IndexdIndexEndpoint = "/index/index"

	// FenceUserEndpoint is the endpoint postfix for FENCE user
	FenceUserEndpoint = "/user/user"

	// FenceDataEndpoint is the endpoint postfix for FENCE data
	FenceDataEndpoint = "/user/data"

	// FenceAccessTokenEndpoint is the endpoint postfix for FENCE access token
	FenceAccessTokenEndpoint = "/user/credentials/api/access_token"

	// FenceDataUploadEndpoint is the endpoint postfix for FENCE data upload
	FenceDataUploadEndpoint = FenceDataEndpoint + "/upload"

	// FenceDataDownloadEndpoint is the endpoint postfix for FENCE data download
	FenceDataDownloadEndpoint = FenceDataEndpoint + "/download"

	// FenceDataMultipartInitEndpoint is the endpoint postfix for FENCE multipart init
	FenceDataMultipartInitEndpoint = FenceDataEndpoint + "/multipart/init"

	// FenceDataMultipartUploadEndpoint is the endpoint postfix for FENCE multipart upload
	FenceDataMultipartUploadEndpoint = FenceDataEndpoint + "/multipart/upload"

	// FenceDataMultipartCompleteEndpoint is the endpoint postfix for FENCE multipart complete
	FenceDataMultipartCompleteEndpoint = FenceDataEndpoint + "/multipart/complete"

	// PathSeparator is os dependent path separator char
	PathSeparator = string(os.PathSeparator)

	// DefaultTimeout is used to set timeout value for http client
	DefaultTimeout = 120 * time.Second

	HeaderContentType   = "Content-Type"
	MIMEApplicationJSON = "application/json"

	// FileSizeLimit is the maximum single file size for non-multipart upload (5GB)
	FileSizeLimit = 5 * GB

	// MultipartFileSizeLimit is the maximum single file size for multipart upload (5TB)
	MultipartFileSizeLimit = 5 * TB
	MinMultipartChunkSize  = 10 * MB

	// MaxRetryCount is the maximum retry number per record
	MaxRetryCount = 5
	MaxWaitTime   = 300

	MaxMultipartParts    = 10000
	MaxConcurrentUploads = 10
	MaxRetries           = 5
)

var (
	// MinChunkSize is configurable via git config and initialized in init()
	MinChunkSize int64
)

func init() {
	v, err := GetLfsCustomTransferInt("lfs.customtransfer.drs.multipart-min-chunk-size", 10)
	if err != nil {
		log.Printf("Warning: Could not read git config for multipart-min-chunk-size, using default (10 MB): %v\n", err)
		MinChunkSize = int64(10) * MB
		return
	}

	MinChunkSize = int64(v) * MB
}

func GetLfsCustomTransferInt(key string, defaultValue int64) (int64, error) {
	defaultText := strconv.FormatInt(defaultValue, 10)
	// TODO cache or get all the configs at once?
	cmd := exec.Command("git", "config", "--get", "--default", defaultText, key)
	output, err := cmd.Output()
	if err != nil {
		return defaultValue, fmt.Errorf("error reading git config %s: %v", key, err)
	}

	value := strings.TrimSpace(string(output))

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue, fmt.Errorf("invalid int value for %s: %q", key, value)
	}

	if parsed < 0 {
		return defaultValue, fmt.Errorf("invalid negative int value for %s: %d", key, parsed)
	}

	if parsed == 0 || parsed > 500 {
		return defaultValue, fmt.Errorf("invalid int value for %s: %d. Must be between 1 and 500", key, parsed)
	}

	return parsed, nil
}
