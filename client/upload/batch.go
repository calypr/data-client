package upload

import (
	"errors"
	"net/http"
	"os"
	"sync"

	client "github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/vbauerster/mpb/v8"
)

func InitBatchUploadChannels(numParallel int, inputSliceLen int) (int, chan *http.Response, chan error, []common.FileUploadRequestObject) {
	getNumberOfWorkers := func(numParallel int, inputSliceLen int) int {
		workers := numParallel
		if workers < 1 || workers > inputSliceLen {
			workers = inputSliceLen
		}
		return workers
	}

	workers := getNumberOfWorkers(numParallel, inputSliceLen)
	respCh := make(chan *http.Response, inputSliceLen)
	errCh := make(chan error, inputSliceLen)
	batchFURSlice := make([]common.FileUploadRequestObject, 0)
	return workers, respCh, errCh, batchFURSlice
}

func BatchUpload(g3i client.Gen3Interface, furObjects []common.FileUploadRequestObject, workers int, respCh chan *http.Response, errCh chan error, bucketName string) {
	progress := mpb.New(mpb.WithOutput(os.Stdout))

	for i := range furObjects {
		if furObjects[i].Bucket == "" {
			furObjects[i].Bucket = bucketName
		}
		if furObjects[i].GUID == "" {
			resp, err := GeneratePresignedURL(g3i, furObjects[i].Filename, furObjects[i].FileMetadata, bucketName)
			if err != nil {
				g3i.Logger().Failed(furObjects[i].FilePath, furObjects[i].Filename, furObjects[i].FileMetadata, resp.GUID, 0, false)
				errCh <- err
				continue
			}
			furObjects[i].PresignedURL = resp.URL
			furObjects[i].GUID = resp.GUID
			// update failed log with new guid
			g3i.Logger().Failed(furObjects[i].FilePath, furObjects[i].Filename, furObjects[i].FileMetadata, resp.GUID, 0, false)
		}
		file, err := os.Open(furObjects[i].FilePath)
		if err != nil {
			g3i.Logger().Failed(furObjects[i].FilePath, furObjects[i].Filename, furObjects[i].FileMetadata, furObjects[i].GUID, 0, false)
			errCh <- errors.New("File open error: " + err.Error())
			continue
		}
		defer file.Close()

		furObjects[i], err = generateUploadRequest(g3i, furObjects[i], file, progress)
		if err != nil {
			file.Close()
			g3i.Logger().Failed(furObjects[i].FilePath, furObjects[i].Filename, furObjects[i].FileMetadata, furObjects[i].GUID, 0, false)
			errCh <- errors.New("Error occurred during request generation: " + err.Error())
			continue
		}
	}

	furObjectCh := make(chan common.FileUploadRequestObject, len(furObjects))

	client := &http.Client{}
	wg := sync.WaitGroup{}
	for range workers {
		wg.Add(1)
		go func() {
			for furObject := range furObjectCh {
				if furObject.Request != nil {
					resp, err := client.Do(furObject.Request)
					if err != nil {
						g3i.Logger().Failed(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, 0, false)
						errCh <- err
					} else {
						if resp.StatusCode != 200 {
							g3i.Logger().Failed(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, 0, false)
						} else {
							respCh <- resp
							g3i.Logger().DeleteFromFailedLog(furObject.FilePath)
							g3i.Logger().Succeeded(furObject.FilePath, furObject.GUID)
							g3i.Logger().Scoreboard().IncrementSB(0)
						}
					}
				} else if furObject.FilePath != "" {
					g3i.Logger().Failed(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, 0, false)
				}
			}
			wg.Done()
		}()
	}

	for i := range furObjects {
		furObjectCh <- furObjects[i]
	}
	close(furObjectCh)

	wg.Wait()
	progress.Wait()
}
