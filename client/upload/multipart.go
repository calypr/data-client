package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

func MultipartUpload(ctx context.Context, g3 client.Gen3Interface, req common.FileUploadRequestObject, file *os.File, showProgress bool) error {
	g3.Logger().Printf("File Upload Request: %#v\n", req)

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
	g3.Logger().Printf("Initialized Upload: ID=%s, Key=%s\n", uploadID, key)

	optimalChunkSize := func(fSize int64) int64 {
		if fSize <= 512*common.MB {
			return 32 * common.MB
		}
		chunkSize := fSize / common.MaxMultipartParts
		if chunkSize < common.MinChunkSize {
			chunkSize = common.MinChunkSize
		}
		return ((chunkSize + common.MB - 1) / common.MB) * common.MB
	}

	chunkSize := optimalChunkSize(fileSize)
	numChunks := int((fileSize + chunkSize - 1) / chunkSize)

	chunks := make(chan int, numChunks)
	for i := 1; i <= numChunks; i++ {
		chunks <- i
	}
	close(chunks)

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		parts        []MultipartPartObject
		uploadErrors []error
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
			parts = append(parts, MultipartPartObject{
				PartNumber: partNum,
				ETag:       etag,
			})
			if bar != nil {
				bar.IncrInt64(size)
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

	g3.Logger().Printf("Successfully uploaded %s to %s", req.Filename, key)
	g3.Logger().Succeeded(req.FilePath, req.GUID)
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
