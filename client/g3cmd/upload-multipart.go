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
	// Instantiate interface to Gen3
	//
	if !enableLogs {
		log.SetOutput(io.Discard)
	}
	gen3Interface := NewGen3Interface()

	// The profileConfig is used by underlying functions, so it must be parsed and set globally.
	var err error
	profileConfig, err = conf.ParseConfig(profile)
	if err != nil {
		return fmt.Errorf("error parsing profile config: %w", err)
	}

	valid, err := conf.IsValidCredential(profileConfig)
	if err != nil && !valid {
		return err
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
	// We pass 0 for the initial retryCount.
	err = multipartUpload(gen3Interface, fileInfo, 0, bucketName)
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

func multipartUpload(g3 Gen3Interface, furObject commonUtils.FileUploadRequestObject, retryCount int, bucketName string) error {
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
	uploadID, guid, err := InitMultipartUpload(g3, furObject, bucketName)
	if err != nil {
		err = fmt.Errorf("FAILED multipart upload for %s: %s", furObject.Filename, err.Error())
		//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)
		return err
	}
	// Update the FURObject's GUID with the one returned from Gen3 (in case it was newly generated)
	// We'll update the failed log with this GUID for consistency
	furObject.GUID = guid
	// haven't figured out a good way to ballance the loggers yet so just comment this out for now.
	//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, retryCount, true, true)

	key := guid + "/" + furObject.Filename
	parts := []MultipartPartObject{}
	numOfWorkers, numOfChunks, chunkSize := calculateChunksAndWorkers(fi.Size())
	chunkIndexCh := make(chan int, numOfChunks)
	var bar *pb.ProgressBar
	// Only use progress bar output if logger is not muted
	if log.Writer() != io.Discard {
		bar = pb.New64(fi.Size()).SetUnits(pb.U_BYTES).SetRefreshRate(time.Millisecond * 10).Prefix(furObject.Filename + " ")
		bar.Start()
	}
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
					//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, guid, retryCount, true, true)
					continue
				}

				// Update log calls inside the worker to use the new guid and furObject data
				var n int
				err = retry(MaxRetryCount, furObject.FilePath, guid, func() (err error) {
					n, err = file.ReadAt(buf[:cap(buf)], int64((chunkIndex-1))*chunkSize)
					buf = buf[:n]
					if err == io.EOF {
						err = nil
					}
					return
				})
				if err != nil {
					//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, guid, retryCount, true, true)
					continue
				}

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
						return
					} else if eTag = resp.Header.Get("ETag"); eTag == "" {
						err = errors.New("No ETag found in header")
						return
					}
					return
				})
				if err != nil {
					//logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, guid, retryCount, true, true)
					continue
				}

				multipartUploadLock.Lock()
				parts = append(parts, (MultipartPartObject{PartNumber: chunkIndex, ETag: eTag}))
				if log.Writer() != io.Discard {
					bar.Add(n)
				}
				multipartUploadLock.Unlock()
			}
			wg.Done()
		}()
	}

	for i := 1; i <= numOfChunks; i++ {
		chunkIndexCh <- i
	}
	close(chunkIndexCh)

	wg.Wait()
	if log.Writer() != io.Discard {
		bar.Finish()
	}
	if len(parts) != numOfChunks {
		err = fmt.Errorf("FAILED multipart upload for %s: Total number of received ETags doesn't match the total number of chunks", furObject.Filename)
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
