package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"

	client "github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	req "github.com/calypr/data-client/client/request"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

/*
func MultipartUpload(ctx context.Context, g3 client.Gen3Interface, req common.FileUploadRequestObject) error {
	// 1. Setup File and Metadata
	file, err := os.Open(req.FilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, _ := file.Stat()
	chunkSize := calculateOptimalChunk(stat.Size())

	// 2. Initialize Upload via API
	uploadID, finalGUID, err := initMultipartUpload(g3, req, req.Bucket)
	if err != nil {
		return err
	}

	// 3. Create a thread-safe Group
	g, ctx := errgroup.WithContext(ctx)
	partsChan := make(chan MultipartPartObject, common.MaxMultipartParts)

	// 4. Worker Pool logic
	// We use a semaphore or fixed number of workers
	workerSem := make(chan struct{}, common.MaxConcurrentUploads)

	for i := 1; i <= numChunks; i++ {
		partNum := i // capture for closure
		g.Go(func() error {
			workerSem <- struct{}{}        // Acquire
			defer func() { <-workerSem }() // Release

			offset := int64(partNum-1) * chunkSize
			size := min(chunkSize, stat.Size()-offset)

			// CRITICAL: NewSectionReader is thread-safe! No Seek needed.
			section := io.NewSectionReader(file, offset, size)

			// 5. Execution Logic
			url, err := generateMultipartPresignedURL(g3, key, uploadID, partNum, req.Bucket)
			if err != nil {
				return err
			}

			etag, err := uploadPart(ctx, g3, url, section)
			if err != nil {
				return err
			}

			partsChan <- MultipartPartObject{PartNumber: partNum, ETag: etag}
			return nil
		})
	}

	// 6. Wait for all workers to finish
	if err := g.Wait(); err != nil {
		return err
	}
	close(partsChan)

	// 7. Collect results, sort, and complete
	return completeUpload(g3, uploadID, partsChan)
}

*/

// MultipartUpload is now clean, context-aware, and uses modern progress bars
func MultipartUpload(ctx context.Context, g3 client.Gen3Interface, req common.FileUploadRequestObject, showProgress bool) error {
	fmt.Println("ENTERING.....")
	g3.Logger().Printf("File Upload Request: %#v\n", req)

	file, err := os.Open(req.FilePath)
	if err != nil {
		fmt.Println("ERR IN OPEN FILE", req.FilePath)
		return fmt.Errorf("cannot open file %s: %w", req.FilePath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		fmt.Println("ERR IN STAT FILE", req.FilePath)
		return fmt.Errorf("cannot stat file: %w", err)
	}

	g3.Logger().Printf("File Name: '%s', File Size: '%d'\n", stat.Name(), stat.Size())

	fileSize := stat.Size()
	if fileSize == 0 {
		return fmt.Errorf("file is empty: %s", req.Filename)
	}

	if fileSize < 5*common.GB {
		g3.Logger().Printf("File size < 5GB (%d bytes), using single-part upload\n", fileSize)
		err := UploadSingleFileWrapper(ctx, g3.GetCredential().Profile, req.GUID, req.FilePath, req.Bucket, showProgress)
		if err != nil {
			g3.Logger().Fatal(err.Error())
		}
		return nil
	}

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
	uploadID, finalGUID, err := initMultipartUpload(ctx, g3, req, req.Bucket)
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
			url, err := generateMultipartPresignedURL(ctx, g3, key, uploadID, partNum, req.Bucket)
			if err != nil {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Errorf("presigned url generation failed for part %d: %w", partNum, err))
				mu.Unlock()
				continue
			}

			uploadPart(ctx, g3, url, reader)

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

	if err := CompleteMultipartUpload(ctx, g3, key, uploadID, parts, req.Bucket); err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	g3.Logger().Printf("Successfully uploaded %s as %s (%d)", req.Filename, finalGUID, stat.Size())
	return nil
}

// InitMultipartUpload helps sending requests to FENCE to init a multipart upload
func initMultipartUpload(ctx context.Context, g3 client.Gen3Interface, furObject common.FileUploadRequestObject, bucketName string) (string, string, error) {
	// Use Filename and GUID directly from the unified request object

	reader, err := common.ToJSONReader(
		InitRequestObject{
			Filename: furObject.Filename,
			Bucket:   bucketName,
			GUID:     furObject.GUID,
		},
	)

	cred := g3.GetCredential()
	resp, err := g3.Do(
		ctx,
		&req.RequestBuilder{
			Method:  http.MethodPost,
			Url:     cred.APIEndpoint + common.FenceDataMultipartInitEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Body:    reader,
			Token:   cred.AccessToken,
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
func generateMultipartPresignedURL(ctx context.Context, g3 client.Gen3Interface, key string, uploadID string, partNumber int, bucketName string) (string, error) {

	reader, err := common.ToJSONReader(
		MultipartUploadRequestObject{
			Key:        key,
			UploadID:   uploadID,
			PartNumber: partNumber,
			Bucket:     bucketName,
		},
	)
	if err != nil {
		return "", err
	}

	cred := g3.GetCredential()
	resp, err := g3.Do(
		ctx,
		&req.RequestBuilder{
			Url:     cred.APIEndpoint + common.FenceDataMultipartUploadEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Method:  http.MethodPost,
			Body:    reader,
			Token:   cred.AccessToken,
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
func CompleteMultipartUpload(ctx context.Context, g3 client.Gen3Interface, key string, uploadID string, parts []MultipartPartObject, bucketName string) error {
	multipartCompleteObject := MultipartCompleteRequestObject{Key: key, UploadID: uploadID, Parts: parts, Bucket: bucketName}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(multipartCompleteObject)
	if err != nil {
		return errors.New("Error occurred during encoding multipart upload data: " + err.Error())
	}

	// TOOD: error check this, return resp information
	cred := g3.GetCredential()
	_, err = g3.Do(
		ctx,
		&req.RequestBuilder{
			Url:     cred.APIEndpoint + common.FenceDataMultipartCompleteEndpoint,
			Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
			Body:    &buf,
			Method:  http.MethodPost,
			Token:   cred.AccessToken,
		},
	)
	if err != nil {
		return errors.New("Error has occurred during completing multipart upload, detailed error message: " + err.Error())
	}
	return nil
}

// GenerateUploadRequest helps preparing the HTTP request for upload and the progress bar for single part upload
func generateUploadRequest(ctx context.Context, g3 client.Gen3Interface, furObject common.FileUploadRequestObject, file *os.File, progress *mpb.Progress) (common.FileUploadRequestObject, error) {
	if furObject.PresignedURL == "" {
		endPointPostfix := common.FenceDataUploadEndpoint + "/" + furObject.GUID + "?file_name=" + url.QueryEscape(furObject.Filename)

		if furObject.Bucket != "" {
			endPointPostfix += "&bucket=" + furObject.Bucket
		}
		cred := g3.GetCredential()
		resp, err := g3.Do(
			ctx,
			&req.RequestBuilder{
				Url:     cred.APIEndpoint + endPointPostfix,
				Headers: map[string]string{common.HeaderContentType: common.MIMEApplicationJSON},
				Token:   cred.AccessToken,
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

	return furObject, err
}

// uploadPart now returns the ETag and error directly.
// It accepts a Context to allow for cancellation (e.g., if another part fails).
func uploadPart(ctx context.Context, g3 client.Gen3Interface, url string, data io.Reader) (string, error) {
	// The AuthTransport handles the tokens; Request handles the retries.
	resp, err := g3.Do(ctx, &req.RequestBuilder{
		Method: http.MethodPut,
		Url:    url,
		Body:   data,
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read a small portion of the error body for debugging,
		// but limit it to prevent memory exhaustion on massive error pages.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(body))
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", errors.New("server did not return an ETag")
	}

	// S3 often returns ETags wrapped in quotes: "abc123xyz"
	// We clean it here once so the rest of the app doesn't have to.
	return strings.Trim(etag, `"`), nil
}
