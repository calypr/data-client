package g3cmd

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/calypr/data-client/client/logs"

	"github.com/spf13/cobra"
)

func handleFailedRetry(g3i client.Gen3Interface, ro common.RetryObject, retryObjCh chan common.RetryObject, err error) {
	logger := g3i.Logger()

	// Record failure in JSON log
	logger.Failed(ro.FilePath, ro.Filename, ro.FileMetadata, ro.GUID, ro.RetryCount, ro.Multipart)

	if err != nil {
		logger.Println("Error:", err)
	}

	if ro.RetryCount < MaxRetryCount {
		retryObjCh <- ro
		return
	}

	// Max retries reached — clean up
	if ro.GUID != "" {
		if msg, err := DeleteRecord(g3i, ro.GUID); err == nil {
			logger.Println(msg)
		} else {
			logger.Println("Cleanup failed:", err)
		}
	}

	// Final failure
	sb, err := logs.FromSBContext(context.Background())
	if err != nil {
		logger.Println(err)
	}
	sb.IncrementSB(MaxRetryCount + 1)

	if len(retryObjCh) == 0 {
		close(retryObjCh)
		logger.Println("Retry channel closed — all done")
	}
}

func retryUpload(g3i client.Gen3Interface, failedLogMap map[string]common.RetryObject) {
	logger := g3i.Logger()
	sb, err := logs.FromSBContext(context.Background())
	if err != nil {
		logger.Println(err)
	}

	if len(failedLogMap) == 0 {
		logger.Println("No failed files to retry.")
		return
	}

	logger.Println("Starting retry-upload...")
	retryObjCh := make(chan common.RetryObject, len(failedLogMap))

	// Load failed entries (skip already succeeded ones)
	for _, ro := range failedLogMap {
		// Simple check: if succeeded log exists and contains this path, skip
		if common.AlreadySucceededFromFile(ro.FilePath) {
			logger.Printf("Already uploaded: %s — skipping\n", ro.FilePath)
			continue
		}
		retryObjCh <- ro
	}

	if len(retryObjCh) == 0 {
		logger.Println("All failed files were already successfully uploaded in a previous run.")
		return
	}

	for ro := range retryObjCh {
		ro.RetryCount++
		logger.Printf("#%d retry — %s\n", ro.RetryCount, ro.FilePath)
		logger.Printf("Waiting %.0f seconds...\n", GetWaitTime(ro.RetryCount).Seconds())
		time.Sleep(GetWaitTime(ro.RetryCount))

		// Optional: delete old record
		if ro.GUID != "" {
			if msg, err := DeleteRecord(g3i, ro.GUID); err == nil {
				logger.Println(msg)
			}
		}

		// Fix missing filename if needed
		if ro.Filename == "" {
			absPath, _ := common.GetAbsolutePath(ro.FilePath)
			ro.Filename = filepath.Base(absPath)
		}

		var err error
		if ro.Multipart {
			// Multipart retry
			req := common.FileUploadRequestObject{
				FilePath: ro.FilePath,
				Filename: ro.Filename,
				GUID:     ro.GUID,
			}
			err = MultipartUpload(context.Background(), g3i, req, ro.Bucket, true)
			if err == nil {
				logger.Succeeded(ro.FilePath, req.GUID)
				sb.IncrementSB(ro.RetryCount - 1) // success on this retry
				continue
			}
		} else {
			// Single-part retry
			var presignedURL, guid string
			presignedURL, guid, err = GeneratePresignedURL(g3i, ro.Filename, ro.FileMetadata, ro.Bucket)
			if err != nil {
				handleFailedRetry(g3i, ro, retryObjCh, err)
				continue
			}

			file, err := os.Open(ro.FilePath)
			if err != nil {
				handleFailedRetry(g3i, ro, retryObjCh, err)
				continue
			}
			stat, _ := file.Stat()
			file.Close()

			if stat.Size() > FileSizeLimit {
				ro.Multipart = true
				retryObjCh <- ro
				continue
			}

			fur := common.FileUploadRequestObject{
				FilePath:     ro.FilePath,
				Filename:     ro.Filename,
				FileMetadata: ro.FileMetadata,
				GUID:         guid,
				PresignedURL: presignedURL,
			}

			fur, err = GenerateUploadRequest(g3i, fur, nil, nil)
			if err != nil {
				handleFailedRetry(g3i, ro, retryObjCh, err)
				continue
			}

			err = uploadFile(g3i, fur, ro.RetryCount)
			if err != nil {
				handleFailedRetry(g3i, ro, retryObjCh, err)
				continue
			}

			// SUCCESS!
			logger.Succeeded(ro.FilePath, fur.GUID)
			sb.IncrementSB(ro.RetryCount - 1)
		}

		if len(retryObjCh) == 0 {
			close(retryObjCh)
		}
	}
}

func init() {
	var failedLogPath, profile string

	var retryUploadCmd = &cobra.Command{
		Use:     "retry-upload",
		Short:   "Retry failed uploads from a failed_log.json",
		Long:    `Re-uploads files listed in a failed log using exponential backoff and progress bars.`,
		Example: `./data-client retry-upload --profile=myprofile --failed-log-path=/path/to/failed_log.json`,
		Run: func(cmd *cobra.Command, args []string) {
			Logger, closer := logs.New(profile,
				logs.WithConsole(),
				logs.WithMessageFile(),
				logs.WithFailedLog(),
				logs.WithSucceededLog(),
			)
			defer closer()

			g3, err := client.NewGen3Interface(context.Background(), profile, Logger)
			if err != nil {
				Logger.Fatalf("Failed to initialize client: %v", err)
			}

			logger := g3.Logger()

			// Create scoreboard with our logger injected
			sb := logs.NewSB(MaxRetryCount, logger)

			// Load failed log
			failedMap, err := common.LoadFailedLog(failedLogPath)
			if err != nil {
				logger.Fatalf("Cannot read failed log: %v", err)
			}

			retryUpload(g3, failedMap)
			sb.PrintSB()
		},
	}

	retryUploadCmd.Flags().StringVar(&profile, "profile", "", "Profile to use")
	retryUploadCmd.MarkFlagRequired("profile")

	retryUploadCmd.Flags().StringVar(&failedLogPath, "failed-log-path", "", "Path to failed_log.json")
	retryUploadCmd.MarkFlagRequired("failed-log-path")

	RootCmd.AddCommand(retryUploadCmd)
}
