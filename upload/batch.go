package upload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/calypr/data-client/common"
	client "github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/request"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

func InitBatchUploadChannels(numParallel int, inputSliceLen int) (int, chan *http.Response, chan error, []common.FileUploadRequestObject) {
	workers := numParallel
	if workers < 1 || workers > inputSliceLen {
		workers = inputSliceLen
	}
	if workers < 1 {
		workers = 1
	}

	respCh := make(chan *http.Response, inputSliceLen)
	errCh := make(chan error, inputSliceLen)
	batchSlice := make([]common.FileUploadRequestObject, 0, workers)

	return workers, respCh, errCh, batchSlice
}

func BatchUpload(
	ctx context.Context,
	g3i client.Gen3Interface,
	furObjects []common.FileUploadRequestObject,
	workers int,
	respCh chan *http.Response,
	errCh chan error,
	bucketName string,
) {
	if len(furObjects) == 0 {
		return
	}

	// Ensure bucket is set
	for i := range furObjects {
		if furObjects[i].Bucket == "" {
			furObjects[i].Bucket = bucketName
		}
	}

	progress := mpb.New(mpb.WithOutput(os.Stdout))

	workCh := make(chan common.FileUploadRequestObject, len(furObjects))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fur := range workCh {
				// --- Ensure presigned URL ---
				if fur.PresignedURL == "" {
					resp, err := GeneratePresignedUploadURL(ctx, g3i, fur.ObjectKey, fur.FileMetadata, fur.Bucket)
					if err != nil {
						g3i.Logger().Failed(fur.SourcePath, fur.ObjectKey, fur.FileMetadata, "", 0, false)
						errCh <- err
						continue
					}
					fur.PresignedURL = resp.URL
					fur.GUID = resp.GUID
					g3i.Logger().Failed(fur.SourcePath, fur.ObjectKey, fur.FileMetadata, resp.GUID, 0, false) // update log
				}

				// --- Open file ---
				file, err := os.Open(fur.SourcePath)
				if err != nil {
					g3i.Logger().Failed(fur.SourcePath, fur.ObjectKey, fur.FileMetadata, fur.GUID, 0, false)
					errCh <- fmt.Errorf("file open error: %w", err)
					continue
				}

				fi, err := file.Stat()
				if err != nil {
					file.Close()
					g3i.Logger().Failed(fur.SourcePath, fur.ObjectKey, fur.FileMetadata, fur.GUID, 0, false)
					errCh <- fmt.Errorf("file stat error: %w", err)
					continue
				}

				if fi.Size() > common.FileSizeLimit {
					file.Close()
					g3i.Logger().Failed(fur.SourcePath, fur.ObjectKey, fur.FileMetadata, fur.GUID, 0, false)
					errCh <- fmt.Errorf("file size exceeds limit: %s", fur.ObjectKey)
					continue
				}

				// --- Progress bar ---
				bar := progress.AddBar(fi.Size(),
					mpb.PrependDecorators(
						decor.Name(fur.ObjectKey+" "),
						decor.CountersKibiByte("% .1f / % .1f"),
					),
					mpb.AppendDecorators(
						decor.Percentage(),
						decor.AverageSpeed(decor.SizeB1024(0), " % .1f"),
					),
				)

				proxyReader := bar.ProxyReader(file)

				// --- Upload using DoAuthenticatedRequest (no manual http.Request!) ---
				resp, err := g3i.Fence().Do(
					ctx,
					&request.RequestBuilder{
						Method: http.MethodPut,
						Url:    fur.PresignedURL,
						Body:   proxyReader,
					},
				)

				// Cleanup
				file.Close()
				bar.Abort(false)

				if err != nil {
					g3i.Logger().Failed(fur.SourcePath, fur.ObjectKey, fur.FileMetadata, fur.GUID, 0, false)
					errCh <- err
					continue
				}

				if resp.StatusCode != http.StatusOK {
					bodyBytes, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					errMsg := fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
					g3i.Logger().Failed(fur.SourcePath, fur.ObjectKey, fur.FileMetadata, fur.GUID, 0, false)
					errCh <- errMsg
					continue
				}

				resp.Body.Close()

				// Success
				respCh <- resp
				g3i.Logger().DeleteFromFailedLog(fur.SourcePath)
				g3i.Logger().Succeeded(fur.SourcePath, fur.GUID)
				g3i.Logger().Scoreboard().IncrementSB(0)
			}
		}()
	}

	for _, obj := range furObjects {
		workCh <- obj
	}
	close(workCh)

	wg.Wait()
	progress.Wait()
}
