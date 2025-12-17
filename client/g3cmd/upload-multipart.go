package g3cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/calypr/data-client/client/logs"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

const (
	minChunkSize         = 5 * 1024 * 1024 // S3 minimum part size
	maxMultipartParts    = 10000
	maxConcurrentUploads = 10
	maxRetries           = 5
)

func NewUploadMultipartCmd() *cobra.Command {
	var (
		filePath   string
		guid       string
		bucketName string
	)

	cmd := &cobra.Command{
		Use:   "upload-multipart",
		Short: "Upload a single file using multipart upload",
		Long: `Uploads a large file to object storage using multipart upload.
This method is resilient to network interruptions and supports resume capability.`,
		Example: `./data-client upload-multipart --profile=myprofile --file-path=./large.bam
./data-client upload-multipart --profile=myprofile --file-path=./data.bam --guid=existing-guid`,
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _ := cmd.Flags().GetString("profile")

			return UploadSingleFile(profile, bucketName, filePath, guid)
		},
	}

	cmd.Flags().StringVar(&filePath, "file-path", "", "Path to the file to upload")
	cmd.Flags().StringVar(&guid, "guid", "", "Optional existing GUID (otherwise generated)")
	cmd.Flags().StringVar(&bucketName, "bucket", "", "Target bucket (defaults to configured DATA_UPLOAD_BUCKET)")

	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("file-path")

	return cmd
}

func UploadSingleFile(profile, bucket, filePath, guid string) error {

	logger, closer := logs.New(profile, logs.WithSucceededLog(), logs.WithFailedLog(), logs.WithScoreboard())
	defer closer()
	g3, err := client.NewGen3Interface(
		context.Background(),
		profile,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize Gen3 interface: %w", err)
	}

	absPath, err := common.GetAbsolutePath(filePath)
	if err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	fileInfo := common.FileUploadRequestObject{
		FilePath:     absPath,
		Filename:     filepath.Base(absPath),
		GUID:         guid,
		FileMetadata: common.FileMetadata{},
	}

	return MultipartUpload(context.TODO(), g3, fileInfo, bucket, true)
}

// MultipartUpload is now clean, context-aware, and uses modern progress bars
func MultipartUpload(ctx context.Context, g3 client.Gen3Interface, req common.FileUploadRequestObject, bucketName string, showProgress bool) error {
	g3.Logger().Printf("File Upload Request: %#v\n", req)

	file, err := os.Open(req.FilePath)
	if err != nil {
		return fmt.Errorf("cannot open file %s: %w", req.FilePath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat file: %w", err)
	}

	g3.Logger().Printf("File Name: '%s', File Size: '%d'\n", stat.Name(), stat.Size())

	if stat.Size() == 0 {
		return fmt.Errorf("file is empty: %s", req.Filename)
	}

	// Initialize multipart upload
	uploadID, finalGUID, err := InitMultipartUpload(g3, req, bucketName)
	if err != nil {
		return fmt.Errorf("failed to initiate multipart upload: %w", err)
	}
	req.GUID = finalGUID // update with server-provided GUID

	key := finalGUID + "/" + req.Filename
	chunkSize := optimalChunkSize(stat.Size())

	numChunks := int((stat.Size() + chunkSize - 1) / chunkSize)
	parts := make([]MultipartPartObject, 0, numChunks)

	// Progress bar setup (modern mpb)
	var p *mpb.Progress
	var bar *mpb.Bar
	if showProgress {
		p = mpb.New(mpb.WithOutput(os.Stdout))
		bar = p.AddBar(stat.Size(),
			mpb.PrependDecorators(
				decor.Name(req.Filename+" "),
				decor.CountersKibiByte("%.1f / %.1f"),
			),
			mpb.AppendDecorators(
				decor.Percentage(),
				decor.AverageSpeed(decor.SizeB1024(0), " % .1f"),
			),
		)
	}

	// Channel for chunk indices
	chunks := make(chan int, numChunks)
	for i := 1; i <= numChunks; i++ {
		chunks <- i
	}
	close(chunks)

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		uploadErrors []error
	)

	worker := func() {
		defer wg.Done()
		buf := make([]byte, chunkSize)

		for partNum := range chunks {
			offset := int64(partNum-1) * chunkSize
			end := offset + chunkSize
			end = min(end, stat.Size())
			size := end - offset

			// Read chunk
			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Errorf("seek failed for part %d: %w", partNum, err))
				mu.Unlock()
				continue
			}
			n, err := io.ReadFull(file, buf[:size])
			if err != nil && err != io.ErrUnexpectedEOF {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Errorf("read failed for part %d: %w", partNum, err))
				mu.Unlock()
				continue
			}

			reader := bytes.NewReader(buf[:n])

			// Get presigned URL + upload with retry
			var etag string
			if err := retryWithBackoff(ctx, maxRetries, func() error {
				url, err := GenerateMultipartPresignedURL(g3, key, uploadID, partNum, bucketName)
				if err != nil {
					return err
				}

				return uploadPart(url, reader, &etag)
			}); err != nil {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Errorf("part %d failed after retries: %w", partNum, err))
				mu.Unlock()
				continue
			}

			// Success
			mu.Lock()
			etag = strings.Trim(etag, `"`)
			parts = append(parts, MultipartPartObject{PartNumber: partNum, ETag: etag})
			g3.Logger().Printf("Appended part %d with ETag %s\n", partNum, etag)
			if bar != nil {
				bar.IncrBy(n)
			}
			mu.Unlock()
		}
	}

	// Launch workers
	for range maxConcurrentUploads {
		wg.Add(1)
		go worker()
	}
	wg.Wait()

	if p != nil {
		p.Wait()
	}

	if len(uploadErrors) > 0 {
		return fmt.Errorf("multipart upload failed: %d parts failed: %v", len(uploadErrors), uploadErrors)
	}

	// Sort parts by PartNumber
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	g3.Logger().Printf("Completing multipart upload with %d parts for file %s\n", len(parts), req.Filename)
	for _, part := range parts {
		g3.Logger().Printf("  Part %d: ETag=%s\n", part.PartNumber, part.ETag)
	}

	if err := CompleteMultipartUpload(g3, key, uploadID, parts, bucketName); err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	g3.Logger().Printf("Successfully uploaded %s as %s (%d)", req.Filename, finalGUID, stat.Size())
	return nil
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

// Choose optimal chunk size
func optimalChunkSize(fileSize int64) int64 {
	if fileSize <= 512*1024*1024 {
		return 32 * 1024 * 1024 // 32MB for smaller files
	}
	chunkSize := max(fileSize/maxMultipartParts, minChunkSize)

	// Round up to nearest MB for cleanliness
	return ((chunkSize + 1024*1024 - 1) / (1024 * 1024)) * 1024 * 1024
}

// Upload single part via presigned URL
func uploadPart(url string, data io.Reader, etagOut *string) error {
	req, err := http.NewRequest(http.MethodPut, url, data)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	*etagOut = resp.Header.Get("ETag")
	if *etagOut == "" {
		return fmt.Errorf("no ETag in response")
	}
	return nil
}
