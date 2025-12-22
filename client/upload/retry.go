package upload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	client "github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
)

// GetWaitTime calculates exponential backoff with cap
func GetWaitTime(retryCount int) time.Duration {
	exp := 1 << retryCount // 2^retryCount
	seconds := int64(exp)
	if seconds > common.MaxWaitTime {
		seconds = common.MaxWaitTime
	}
	return time.Duration(seconds) * time.Second
}

// RetryFailedUploads re-uploads previously failed files with exponential backoff
func RetryFailedUploads(g3 client.Gen3Interface, failedMap map[string]common.RetryObject) {
	logger := g3.Logger()
	if len(failedMap) == 0 {
		logger.Println("No failed files to retry.")
		return
	}

	sb := logger.Scoreboard()
	if sb == nil {
		var err error
		sb, err = logs.FromSBContext(context.Background())
		if err != nil {
			logger.Println("Scoreboard unavailable:", err)
		}
	}

	logger.Println("Starting retry-upload...")
	retryChan := make(chan common.RetryObject, len(failedMap))

	// Queue only non-already-succeeded files
	for _, ro := range failedMap {
		if common.AlreadySucceededFromFile(ro.FilePath) {
			logger.Printf("Already uploaded successfully in another run: %s — skipping\n", ro.FilePath)
			continue
		}
		retryChan <- ro
	}

	if len(retryChan) == 0 {
		logger.Println("All previously failed files have since succeeded.")
		return
	}

	for ro := range retryChan {
		ro.RetryCount++
		logger.Printf("#%d retry — %s\n", ro.RetryCount, ro.FilePath)
		wait := GetWaitTime(ro.RetryCount)
		logger.Printf("Waiting %.0f seconds before retry...\n", wait.Seconds())
		time.Sleep(wait)

		// Clean up old record if exists
		if ro.GUID != "" {
			if msg, err := g3.DeleteRecord(g3.GetCredential(), ro.GUID); err == nil {
				logger.Println(msg)
			}
		}

		// Ensure filename is set
		if ro.Filename == "" {
			absPath, _ := common.GetAbsolutePath(ro.FilePath)
			ro.Filename = filepath.Base(absPath)
		}

		var err error
		if ro.Multipart {
			// Retry multipart
			req := common.FileUploadRequestObject{
				FilePath:     ro.FilePath,
				Filename:     ro.Filename,
				GUID:         ro.GUID,
				FileMetadata: ro.FileMetadata,
				Bucket:       ro.Bucket,
			}
			err = MultipartUpload(context.Background(), g3, req, true)
			if err == nil {
				logger.Succeeded(ro.FilePath, req.GUID)
				if sb != nil {
					sb.IncrementSB(ro.RetryCount - 1)
				}
				continue
			}
		} else {
			// Retry single-part
			respObj, err := GeneratePresignedURL(g3, ro.Filename, ro.FileMetadata, ro.Bucket)
			if err != nil {
				handleRetryFailure(g3, ro, retryChan, err)
				continue
			}

			file, err := os.Open(ro.FilePath)
			if err != nil {
				handleRetryFailure(g3, ro, retryChan, err)
				continue
			}
			stat, _ := file.Stat()
			file.Close()

			if stat.Size() > common.FileSizeLimit {
				ro.Multipart = true
				retryChan <- ro
				continue
			}

			fur := common.FileUploadRequestObject{
				FilePath:     ro.FilePath,
				Filename:     ro.Filename,
				FileMetadata: ro.FileMetadata,
				GUID:         respObj.GUID,
				PresignedURL: respObj.URL,
			}

			fur, err = generateUploadRequest(g3, fur, nil, nil)
			if err != nil {
				handleRetryFailure(g3, ro, retryChan, err)
				continue
			}

			err = UploadSingleFile(g3, fur, true)
			if err == nil {
				logger.Succeeded(ro.FilePath, fur.GUID)
				if sb != nil {
					sb.IncrementSB(ro.RetryCount - 1)
				}
				continue
			}
		}

		// On failure, requeue if retries remain
		handleRetryFailure(g3, ro, retryChan, err)
	}
}

// handleRetryFailure logs failure and requeues if retries remain
func handleRetryFailure(g3 client.Gen3Interface, ro common.RetryObject, retryChan chan common.RetryObject, err error) {
	logger := g3.Logger()
	logger.Failed(ro.FilePath, ro.Filename, ro.FileMetadata, ro.GUID, ro.RetryCount, ro.Multipart)
	if err != nil {
		logger.Println("Retry error:", err)
	}

	if ro.RetryCount < common.MaxRetryCount {
		retryChan <- ro
		return
	}

	// Max retries reached — final cleanup
	if ro.GUID != "" {
		if msg, err := g3.DeleteRecord(g3.GetCredential(), ro.GUID); err == nil {
			logger.Println("Cleaned up failed record:", msg)
		} else {
			logger.Println("Cleanup failed:", err)
		}
	}

	if sb := logger.Scoreboard(); sb != nil {
		sb.IncrementSB(common.MaxRetryCount + 1)
	}

	if len(retryChan) == 0 {
		close(retryChan)
	}
}

// Helper: exponential backoff retry
func retryWithBackoff(ctx context.Context, attempts int, fn func() error) error {
	var err error
	for i := range attempts {
		if err = fn(); err == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoffDuration(i)):
		}
	}
	return fmt.Errorf("after %d attempts: %w", attempts, err)
}

func backoffDuration(attempt int) time.Duration {
	return min(time.Duration(1<<uint(attempt))*200*time.Millisecond, 10*time.Second)
}
