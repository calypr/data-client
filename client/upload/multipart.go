package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// InitMultipartUpload, GenerateMultipartPresignedURL, CompleteMultipartUpload moved here similarly...

// MultipartUpload is now clean, context-aware, and uses modern progress bars
func MultipartUpload(ctx context.Context, g3 client.Gen3Interface, req common.FileUploadRequestObject, showProgress bool) error {
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

	fileSize := stat.Size()
	if fileSize == 0 {
		return fmt.Errorf("file is empty: %s", req.Filename)
	}

	if fileSize < 5*common.GB {
		g3.Logger().Printf("File size < 5GB (%d bytes), using single-part upload\n", fileSize)
		err := UploadSingleFileWrapper(g3.GetCredential().Profile, req.GUID, req.FilePath, req.Bucket, showProgress)
		if err != nil {
			g3.Logger().Fatal(err.Error())
		}
		return nil
	}

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

	// Initialize multipart upload
	uploadID, finalGUID, err := initMultipartUpload(g3, req, req.Bucket)
	if err != nil {
		return fmt.Errorf("failed to initiate multipart upload: %w", err)
	}
	req.GUID = finalGUID // update with server-provided GUID

	// Define closure since this function is only used here
	optimalChunkSize := func(fileSize int64) int64 {
		// Define internal constants or use variables from the outer scope

		if fileSize <= 512*common.MB {
			return 32 * common.MB // 32MB for smaller files
		}

		chunkSize := fileSize / common.MaxMultipartParts
		if chunkSize < common.MinChunkSize {
			chunkSize = common.MinChunkSize
		}

		// Round up to nearest MB for cleanliness
		return ((chunkSize + common.MB - 1) / common.MB) * common.MB
	}

	key := finalGUID + "/" + req.Filename
	chunkSize := optimalChunkSize(stat.Size())

	numChunks := int((stat.Size() + chunkSize - 1) / chunkSize)
	parts := make([]MultipartPartObject, 0, numChunks)

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
			if err := retryWithBackoff(ctx, common.MaxRetries, func() error {
				url, err := generateMultipartPresignedURL(g3, key, uploadID, partNum, req.Bucket)
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
	for range common.MaxConcurrentUploads {
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

	if err := CompleteMultipartUpload(g3, key, uploadID, parts, req.Bucket); err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	g3.Logger().Printf("Successfully uploaded %s as %s (%d)", req.Filename, finalGUID, stat.Size())
	return nil
}
