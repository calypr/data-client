package upload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"sync/atomic"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/fence"
	client "github.com/calypr/data-client/g3client"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

func MultipartUpload(ctx context.Context, g3 client.Gen3Interface, req common.FileUploadRequestObject, file *os.File, showProgress bool) error {
	g3.Logger().Info("File Upload Request", "request", req)

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat file: %w", err)
	}

	fileSize := stat.Size()
	if fileSize == 0 {
		return fmt.Errorf("file is empty: %s", req.Filename)
	}

	var p *mpb.Progress
	var bar *mpb.Bar
	if showProgress {
		p = mpb.New(mpb.WithOutput(os.Stdout))
		bar = p.AddBar(fileSize,
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

	// 1. Initialize multipart upload
	uploadID, finalGUID, err := initMultipartUpload(ctx, g3, req, req.Bucket)
	if err != nil {
		return fmt.Errorf("failed to initiate multipart upload: %w", err)
	}

	// 2. Construct the S3 Key correctly
	// Ensure finalGUID is not empty to avoid a leading slash
	key := fmt.Sprintf("%s/%s", finalGUID, req.Filename)
	g3.Logger().Info("Initialized Upload", "id", uploadID, "key", key)

	chunkSize := OptimalChunkSize(fileSize)

	numChunks := int((fileSize + chunkSize - 1) / chunkSize)

	chunks := make(chan int, numChunks)
	for i := 1; i <= numChunks; i++ {
		chunks <- i
	}
	close(chunks)

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		parts        []fence.MultipartPart
		uploadErrors []error
		totalBytes   int64 // Atomic counter for monotonically increasing BytesSoFar
	)

	// 3. Worker logic
	worker := func() {
		defer wg.Done()

		for partNum := range chunks {

			offset := int64(partNum-1) * chunkSize
			size := chunkSize
			if offset+size > fileSize {
				size = fileSize - offset
			}

			// SectionReader implements io.Reader, io.ReaderAt, and io.Seeker
			// It allows each worker to read its own segment without a shared buffer.
			section := io.NewSectionReader(file, offset, size)

			url, err := generateMultipartPresignedURL(ctx, g3, key, uploadID, partNum, req.Bucket)
			if err != nil {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Errorf("URL generation failed part %d: %w", partNum, err))
				mu.Unlock()
				return
			}

			// Perform the upload using the section directly
			etag, err := uploadPart(ctx, url, section, size)
			if err != nil {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Errorf("upload failed part %d: %w", partNum, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			parts = append(parts, fence.MultipartPart{
				PartNumber: partNum,
				ETag:       etag,
			})
			if bar != nil {
				bar.IncrInt64(size)
			}
			if req.Progress != nil {
				currentTotal := atomic.AddInt64(&totalBytes, size)
				err = req.Progress(common.ProgressEvent{
					Event:          "progress",
					Oid:            resolveUploadOID(req),
					BytesSinceLast: size,
					BytesSoFar:     currentTotal,
				})
				if err != nil {
					g3.Logger().Printf("progress callback error: %v", err)
				}
			}
			mu.Unlock()
		}
	}

	// Launch workers
	for range common.MaxConcurrentUploads {
		wg.Add(1)
		go worker()
	}
	wg.Wait()

	if p != nil {
		p.Wait()
	}

	if len(uploadErrors) > 0 {
		return fmt.Errorf("multipart upload failed with %d errors: %v", len(uploadErrors), uploadErrors)
	}

	// 5. Finalize the upload
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	if err := CompleteMultipartUpload(ctx, g3, key, uploadID, parts, req.Bucket); err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	g3.Logger().Info("Successfully uploaded", "file", req.Filename, "key", key)
	g3.Logger().Succeeded(req.FilePath, req.GUID)
	return nil
}

func initMultipartUpload(ctx context.Context, g3 client.Gen3Interface, furObject common.FileUploadRequestObject, bucketName string) (string, string, error) {
	msg, err := g3.Fence().InitMultipartUpload(ctx, furObject.Filename, bucketName, furObject.GUID)

	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return "", "", errors.New(err.Error() + "\nPlease check to ensure FENCE version is at 2.8.0 or beyond")
		}
		return "", "", errors.New("Error has occurred during multipart upload initialization, detailed error message: " + err.Error())
	}

	if msg.UploadID == "" || msg.GUID == "" {
		return "", "", errors.New("unknown error has occurred during multipart upload initialization. Please check logs from Gen3 services")
	}
	return msg.UploadID, msg.GUID, nil
}

func generateMultipartPresignedURL(ctx context.Context, g3 client.Gen3Interface, key string, uploadID string, partNumber int, bucketName string) (string, error) {
	url, err := g3.Fence().GenerateMultipartPresignedURL(ctx, key, uploadID, partNumber, bucketName)
	if err != nil {
		return "", errors.New("Error has occurred during multipart upload presigned url generation, detailed error message: " + err.Error())
	}

	if url == "" {
		return "", errors.New("unknown error has occurred during multipart upload presigned url generation. Please check logs from Gen3 services")
	}
	return url, nil
}

func CompleteMultipartUpload(ctx context.Context, g3 client.Gen3Interface, key string, uploadID string, parts []fence.MultipartPart, bucketName string) error {
	err := g3.Fence().CompleteMultipartUpload(ctx, key, uploadID, parts, bucketName)
	if err != nil {
		return errors.New("Error has occurred during completing multipart upload, detailed error message: " + err.Error())
	}
	return nil
}

// uploadPart now returns the ETag and error directly.
// It accepts a Context to allow for cancellation (e.g., if another part fails).
func uploadPart(ctx context.Context, url string, data io.Reader, partSize int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, data)
	if err != nil {
		return "", err
	}

	req.ContentLength = partSize

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("upload failed (%d): %s", resp.StatusCode, body)
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", errors.New("no ETag returned")
	}

	return strings.Trim(etag, `"`), nil
}
