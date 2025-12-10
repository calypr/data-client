package g3cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/calypr/data-client/client/commonUtils"
	"github.com/calypr/data-client/client/logs"
	"github.com/spf13/cobra"
	pb "gopkg.in/cheggaaa/pb.v1"
)

func init() {
	var bucketName string
	var filePath string
	var guid string

	var uploadMultipartCmd = &cobra.Command{
		Use:   "upload-multipart",
		Short: "Upload a single file to object storage using multipart upload.",
		Long:  `Uploads a single file using the multipart upload strategy, which is preferred for large files due to improved resilience and retry capabilities.`,
		Example: "Upload a file and get a new GUID:\n./data-client upload-multipart --profile=<profile-name> --file-path=<path-to-file/data.bam>\n" +
			"Upload a file using a pre-existing GUID:\n./data-client upload-multipart --profile=<profile-name> --file-path=<path-to-file/data.bam> --guid=<existing-guid>",
		Run: func(cmd *cobra.Command, args []string) {
			logs.InitSucceededLog(profile)
			logs.InitFailedLog(profile)
			logs.SetToBoth()
			logs.InitScoreBoard(MaxRetryCount)
			err := UploadSingleMultipart(profile, filePath, bucketName, guid, true)
			if err != nil {
				log.Fatalf("Multipart upload failed: %v", err)
			}
			logs.PrintScoreBoard()
			logs.CloseAll()
		},
	}

	uploadMultipartCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	uploadMultipartCmd.MarkFlagRequired("profile") //nolint:errcheck
	uploadMultipartCmd.Flags().StringVar(&filePath, "file-path", "", "The path to the single file to be uploaded")
	uploadMultipartCmd.MarkFlagRequired("file-path") //nolint:errcheck
	uploadMultipartCmd.Flags().StringVar(&guid, "guid", "", "Optional: A pre-existing GUID to associate with the uploaded file. If empty, Gen3 will generate a new one.")
	uploadMultipartCmd.Flags().StringVar(&bucketName, "bucket", "", "The bucket to which the file will be uploaded. If not provided, defaults to Gen3's configured DATA_UPLOAD_BUCKET.")
	RootCmd.AddCommand(uploadMultipartCmd)
}

var multipartUploadLock sync.Mutex

// UploadSingleMultipart uploads a single file to Gen3 using the multipart upload strategy.
// This is the preferred method for large files as it provides resilience through retries on a per-chunk basis.
func UploadSingleMultipart(profile string, filePath string, bucketName string, guid string, enableLogs bool) error {
	if !enableLogs {
		log.SetOutput(io.Discard)
	}
	// Instantiate interface to Gen3
	gen3Interface, err := NewGen3Interface(profile)
	if err != nil {
		return fmt.Errorf("error parsing profile config: %w", err)
	}

	// Validate the file path to ensure it points to a single, existing file.
	filePaths, err := commonUtils.ParseFilePaths(filePath, false)
	if err != nil {
		return fmt.Errorf("file path parsing error: %w", err)
	}
	if len(filePaths) != 1 {
		return fmt.Errorf("path must resolve to a single file, but found %d", len(filePaths))
	}

	absFilePath := filePaths[0]
	if _, err := os.Stat(absFilePath); os.IsNotExist(err) {
		return fmt.Errorf("the file specified \"%s\" does not exist", absFilePath)
	}

	// Create the FileUploadRequestObject struct required by the multipartUpload function.
	fileInfo := commonUtils.FileUploadRequestObject{
		FilePath:     absFilePath,
		Filename:     filepath.Base(absFilePath),
		FileMetadata: commonUtils.FileMetadata{},
		GUID:         guid,
	}

	// Call the existing, robust multipartUpload function to perform the upload.
	// This function handles all the complex logic of chunking, concurrency, API calls, and retries.
	// We pass 0 for the initial retryCount and true to show progress bar for interactive use.
	err = MultipartUpload(gen3Interface, fileInfo, 0, bucketName, true)
	if err != nil {
		// The underlying function will have already logged the specifics.
		// We return a clean error to the caller.
		return fmt.Errorf("multipart upload failed for %s: %w", absFilePath, err)
	}

	// The `multipartUpload` function prints its own success message upon completion.
	return nil
}

func retry(attempts int, filePath string, guid string, f func() error) (err error) {
	for i := 0; ; i++ {
		err = f()
		if err == nil {
			return
		}

		if i >= (attempts - 1) {
			break
		}

		time.Sleep(GetWaitTime(i))

		//log.Println("Retrying after error: ", err)
	}
	return fmt.Errorf("After %d attempts, last error: %s", attempts, err)
}

func MultipartUpload(g3 Gen3Interface, furObject commonUtils.FileUploadRequestObject, retryCount int, bucketName string, showProgressBar bool) error {
	// Use furObject.FilePath
	file, err := os.Open(furObject.FilePath)
	if err != nil {
		err = fmt.Errorf("FAILED multipart upload for %s due to file open error: %s", furObject.FilePath, err.Error())
		//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)
		return err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		err = fmt.Errorf("FAILED multipart upload for %s: file stat error, file may be missing or unreadable because of permissions", furObject.Filename)
		//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)
		return err
	}
	if fi.Size() == 0 {
		err = fmt.Errorf("FAILED multipart upload for %s: the file size must be greater than 0", fi.Name())
		//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)
		return err
	}
	if fi.Size() > MultipartFileSizeLimit {
		err = fmt.Errorf("FAILED multipart upload for %s: the file size has exceeded the limit allowed and cannot be uploaded. The maximum allowed file size is %s", fi.Name(), FormatSize(MultipartFileSizeLimit))
		//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)
		return err
	}
	// Use the refactored InitMultipartUpload with the unified object
	log.Printf("[DEBUG] Calling InitMultipartUpload with GUID: %s, Filename: %s, Bucket: %s", furObject.GUID, furObject.Filename, bucketName)
	uploadID, guid, err := InitMultipartUpload(g3, furObject, bucketName)
	if err != nil {
		err = fmt.Errorf("FAILED multipart upload for %s: %s", furObject.Filename, err.Error())
		//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)
		return err
	}
	// Update the FURObject's GUID with the one returned from Gen3 (in case it was newly generated)
	// We'll update the failed log with this GUID for consistency
	log.Printf("[DEBUG] InitMultipartUpload returned - UploadID: %s, GUID: %s", uploadID, guid)
	if furObject.GUID != "" && guid != furObject.GUID {
		log.Printf("[WARNING] GUID mismatch! Requested: %s, Fence returned: %s", furObject.GUID, guid)
	}
	furObject.GUID = guid
	// haven't figured out a good way to ballance the loggers yet so just comment this out for now.
	//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)

	key := guid + "/" + furObject.Filename
	parts := []MultipartPartObject{}
	numOfWorkers, numOfChunks, chunkSize := calculateChunksAndWorkers(fi.Size())
	chunkIndexCh := make(chan int, numOfChunks)

	// Log chunk calculation details for debugging
	log.Printf("[DEBUG] Multipart upload for %s (size: %d bytes, %s):", furObject.Filename, fi.Size(), FormatSize(fi.Size()))
	log.Printf("[DEBUG]   S3 Key: %s", key)
	log.Printf("[DEBUG]   Workers: %d, Chunks: %d, ChunkSize: %d bytes (%s)", numOfWorkers, numOfChunks, chunkSize, FormatSize(chunkSize))

	var bar *pb.ProgressBar
	// Only show progress bar if explicitly enabled (for interactive use)
	// Progress bars with ANSI codes pollute log files with duplicate lines
	if showProgressBar && log.Writer() != io.Discard {
		bar = pb.New64(fi.Size()).SetUnits(pb.U_BYTES).SetRefreshRate(time.Millisecond * 10).Prefix(furObject.Filename + " ")
		bar.Output = log.Writer()
		bar.Start()
	}
	// Track failed chunks for better error reporting
	var failedChunks []int
	var failedChunksMutex = struct {
		sync.Mutex
		reasons map[int]error
	}{reasons: make(map[int]error)}

	wg := sync.WaitGroup{}
	for i := 0; i < numOfWorkers; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, chunkSize)
			for chunkIndex := range chunkIndexCh {
				var presignedURL string
				// Use furObject.FilePath and new guid for logging
				err = retry(MaxRetryCount, furObject.FilePath, guid, func() (err error) {
					presignedURL, err = GenerateMultipartPresignedURL(g3, key, uploadID, chunkIndex, bucketName)
					return
				})
				if err != nil {
					log.Printf("[ERROR] Chunk %d/%d failed to get presigned URL after %d retries: %v", chunkIndex, numOfChunks, MaxRetryCount, err)
					failedChunksMutex.Lock()
					failedChunks = append(failedChunks, chunkIndex)
					failedChunksMutex.reasons[chunkIndex] = err // record last error for that chunk
					failedChunksMutex.Unlock()
					//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, guid, retryCount, true, true)
					continue
				}

				// Log the presigned URL for debugging 403 errors
				// Sanitize sensitive query parameters but keep the key structure visible
				log.Printf("[DEBUG] Chunk %d/%d: Presigned URL: %s", chunkIndex, numOfChunks, presignedURL)

				// Update log calls inside the worker to use the new guid and furObject data
				var n int
				offset := int64((chunkIndex - 1)) * chunkSize
				err = retry(MaxRetryCount, furObject.FilePath, guid, func() (err error) {
					n, err = file.ReadAt(buf[:cap(buf)], offset)
					buf = buf[:n]
					if err == io.EOF {
						err = nil
					}
					return
				})
				if err != nil {
					log.Printf("[ERROR] Chunk %d/%d failed to read at offset %d after %d retries: %v", chunkIndex, numOfChunks, offset, MaxRetryCount, err)
					failedChunksMutex.Lock()
					failedChunks = append(failedChunks, chunkIndex)
					failedChunksMutex.Unlock()
					//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, guid, retryCount, true, true)
					continue
				}

				log.Printf("[DEBUG] Chunk %d/%d: read %d bytes at offset %d", chunkIndex, numOfChunks, n, offset)

				var eTag string
				err = retry(MaxRetryCount, furObject.FilePath, guid, func() (err error) {
					req, err := http.NewRequest(http.MethodPut, presignedURL, bytes.NewReader(buf))
					if err != nil {
						err = errors.New("Error occurred when creating HTTP request: " + err.Error())
						return
					}
					req.ContentLength = int64(n)
					client := &http.Client{}
					resp, err := client.Do(req)
					if err != nil {
						err = errors.New("Error occurred during upload: " + err.Error())
						return
					}
					if resp.StatusCode != 200 {
						err = errors.New("Upload request got a non-200 response with status code " + strconv.Itoa(resp.StatusCode))

						// also debug here if anything else from the response is useful
						bodyBytes, readErr := io.ReadAll(resp.Body)
						if readErr == nil {
							log.Printf("[DEBUG] Chunk %d/%d: Non-200 response body: %s", chunkIndex, numOfChunks, string(bodyBytes))
						} else {
							log.Printf("[DEBUG] Chunk %d/%d: Failed to read response body: %v", chunkIndex, numOfChunks, readErr)
						}

						return
					} else if eTag = resp.Header.Get("ETag"); eTag == "" {
						err = errors.New("No ETag found in header")
						return
					}
					// Normalize ETag by trimming quotes (S3 returns quoted ETags)
					eTag = strings.Trim(eTag, `"`)
					return
				})
				if err != nil {
					log.Printf("[ERROR] Chunk %d/%d failed to upload (%d bytes) after %d retries: %v", chunkIndex, numOfChunks, n, MaxRetryCount, err)
					failedChunksMutex.Lock()
					failedChunks = append(failedChunks, chunkIndex)
					failedChunksMutex.Unlock()
					//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, guid, retryCount, true, true)
					continue
				}

				log.Printf("[DEBUG] Chunk %d/%d: uploaded successfully, ETag=%s", chunkIndex, numOfChunks, eTag)

				multipartUploadLock.Lock()
				parts = append(parts, (MultipartPartObject{PartNumber: chunkIndex, ETag: eTag}))
				log.Printf("[DEBUG] Chunk %d/%d: added to parts list (total parts now: %d)", chunkIndex, numOfChunks, len(parts))
				if bar != nil {
					bar.Add(n)
				}
				multipartUploadLock.Unlock()
			}
			wg.Done()
		}()
	}

	log.Printf("[DEBUG] Queuing %d chunks for upload (indices 1 to %d)", numOfChunks, numOfChunks)
	for i := 1; i <= numOfChunks; i++ {
		chunkIndexCh <- i
	}
	close(chunkIndexCh)

	wg.Wait()
	if bar != nil {
		bar.Finish()
	}

	log.Printf("[DEBUG] Upload complete: received %d ETags, expected %d chunks", len(parts), numOfChunks)

	if len(parts) != numOfChunks {
		// build list of missing indices
		got := make(map[int]bool)
		for _, p := range parts {
			got[p.PartNumber] = true
		}
		missing := []int{}
		for i := 1; i <= numOfChunks; i++ {
			if !got[i] {
				missing = append(missing, i)
			}
		}
		// build a combined error message with missing chunk numbers and reasons if present
		var reasons []string
		failedChunksMutex.Lock()
		for _, idx := range missing {
			if r, ok := failedChunksMutex.reasons[idx]; ok {
				reasons = append(reasons, fmt.Sprintf("chunk %d: %v", idx, r))
			} else {
				reasons = append(reasons, fmt.Sprintf("chunk %d: unknown error", idx))
			}
		}
		failedChunksMutex.Unlock()
		log.Printf("FAILED multipart upload for %s: Total number of received ETags doesn't match the total number of chunks; missing chunks: %v; reasons: %s", furObject.Filename, missing, strings.Join(reasons, "; "))

		// Log which parts we actually received
		receivedParts := make([]int, len(parts))
		for i, part := range parts {
			receivedParts[i] = part.PartNumber
		}
		log.Printf("[DEBUG] Received parts: %v", receivedParts)

		if len(failedChunks) > 0 {
			sort.Ints(failedChunks)
			err = fmt.Errorf("FAILED multipart upload for %s: Total number of received ETags (%d) doesn't match the total number of chunks (%d). Failed chunks after %d retries: %v",
				furObject.Filename, len(parts), numOfChunks, MaxRetryCount, failedChunks)
		} else {
			err = fmt.Errorf("FAILED multipart upload for %s: Total number of received ETags (%d) doesn't match the total number of chunks (%d). No explicit failures recorded - this may indicate a race condition or buffer reuse issue. Received parts: %v",
				furObject.Filename, len(parts), numOfChunks, receivedParts)
		}
		//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)
		return err
	}

	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber // sort parts in ascending order
	})

	if err = CompleteMultipartUpload(g3, key, uploadID, parts, bucketName); err != nil {
		err = fmt.Errorf("FAILED multipart upload for %s: %s", furObject.Filename, err.Error())
		//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)
		return err
	}

	// Successful upload cleanup
	//logs.DeleteFromFailedLog(furObject.FilePath, true)
	//logs.WriteToSucceededLog(furObject.FilePath, furObject.GUID, true)
	return nil
}
