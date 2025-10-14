package g3cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/calypr/data-client/client/commonUtils"
	"github.com/calypr/data-client/client/logs"
	"github.com/spf13/cobra"
)

func init() {
	var bucketName string
	var includeSubDirName bool
	var uploadPath string
	var batch bool
	var forceMultipart bool
	var numParallel int
	var hasMetadata bool
	var uploadCmd = &cobra.Command{
		Use:   "upload",
		Short: "Upload file(s) to object storage.",
		Long:  `Gets a presigned URL for each file and then uploads the specified file(s).`,
		Example: "For uploading a single file:\n./data-client upload --profile=<profile-name> --upload-path=<path-to-files/data.bam>\n" +
			"For uploading all files within an folder:\n./data-client upload --profile=<profile-name> --upload-path=<path-to-files/folder/>\n" +
			"Can also support regex such as:\n./data-client upload --profile=<profile-name> --upload-path=<path-to-files/folder/*>\n" +
			"Or:\n./data-client upload --profile=<profile-name> --upload-path=<path-to-files/*/folder/*.bam>\n" +
			"This command can also upload file metadata using the --metadata flag. If the --metadata flag is passed, the data-client will look for a file called [filename]_metadata.json in the same folder, which contains the metadata to upload.\n" +
			"For example, if uploading the file `folder/my_file.bam`, the data-client will look for a metadata file at `folder/my_file_metadata.json`.\n" +
			"For the format of the metadata files, see the README.",
		Run: func(cmd *cobra.Command, args []string) {
			// initialize transmission logs
			logs.InitSucceededLog(profile)
			logs.InitFailedLog(profile)
			logs.SetToBoth()
			logs.InitScoreBoard(MaxRetryCount)

			// Instantiate interface to Gen3
			gen3Interface := NewGen3Interface()
			var err error
			profileConfig, err = conf.ParseConfig(profile)
			if err != nil {
				log.Println(err.Error())
				return
			}

			valid, err := conf.IsValidCredential(profileConfig)
			if err != nil && valid {
				log.Println(err)
			} else if !valid {
				log.Fatal(err)
			}

			if hasMetadata {
				hasShepherd, err := gen3Interface.CheckForShepherdAPI(&profileConfig)
				if err != nil {
					log.Printf("WARNING: Error when checking for Shepherd API: %v", err)
				} else {
					if !hasShepherd {
						log.Fatalf("ERROR: Metadata upload (`--metadata`) is not supported in the environment you are uploading to. Double check that you are uploading to the right profile.")
					}
				}
			}

			uploadPath, _ = commonUtils.GetAbsolutePath(uploadPath)
			filePaths, err := commonUtils.ParseFilePaths(uploadPath, hasMetadata)
			if err != nil {
				log.Fatalf("Error when parsing file paths: %s", err.Error())
			}
			uploadRequestObjects := make([]commonUtils.FileUploadRequestObject, 0, len(filePaths))

			fmt.Println("\nThe following file(s) has been found in path \"" + uploadPath + "\" and will be uploaded:")
			for _, filePath := range filePaths {
				// Use ProcessFilename to create the unified object (GUID is empty here, as this command requests a new GUID)
				// ProcessFilename signature: (uploadPath, filePath, objectId, includeSubDirName, includeMetadata)
				furObject, err := ProcessFilename(uploadPath, filePath, "", includeSubDirName, hasMetadata)

				// Handle case where ProcessFilename fails (e.g., metadata parsing error)
				if err != nil {
					// Use the data available for logging the failure
					logs.AddToFailedLog(filePath, filepath.Base(filePath), commonUtils.FileMetadata{}, "", 0, false, true)
					log.Println("Error processing file path or metadata: " + err.Error())
					continue
				}

				// Optional: Display file path before proceeding
				file, _ := os.Open(filePath)
				if fi, _ := file.Stat(); !fi.IsDir() {
					fmt.Println("\t" + filePath)
				}
				file.Close()

				uploadRequestObjects = append(uploadRequestObjects, furObject)
			}
			fmt.Println()

			if len(uploadRequestObjects) == 0 {
				log.Println("No valid file upload requests were created.")
				return
			}

			singlePartObjects, multipartObjects := separateSingleAndMultipartUploads(uploadRequestObjects, forceMultipart)
			if batch {
				workers, respCh, errCh, batchFURObjects := initBatchUploadChannels(numParallel, len(singlePartObjects))

				for _, furObject := range singlePartObjects {
					if len(batchFURObjects) < workers {
						batchFURObjects = append(batchFURObjects, furObject)
					} else {
						batchUpload(gen3Interface, batchFURObjects, workers, respCh, errCh, bucketName)
						batchFURObjects = []commonUtils.FileUploadRequestObject{furObject}
					}
				}
				if len(batchFURObjects) > 0 {
					batchUpload(gen3Interface, batchFURObjects, workers, respCh, errCh, bucketName)
				}

				if len(errCh) > 0 {
					close(errCh)
					for err := range errCh {
						if err != nil {
							log.Printf("Error occurred during uploading: %s\n", err.Error())
						}
					}
				}
			} else {
				for _, furObject := range singlePartObjects {
					file, err := os.Open(furObject.FilePath)
					if err != nil {
						logs.AddToFailedLog(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, 0, false, true)
						log.Println("File open error: " + err.Error())
						continue
					}
					startSingleFileUpload(gen3Interface, furObject, file, bucketName)
				}
			}

			if len(multipartObjects) > 0 {
				err := processMultipartUpload(gen3Interface, multipartObjects, bucketName, includeSubDirName, uploadPath)
				if err != nil {
					log.Println(err.Error())
				}
			}
			if !logs.IsFailedLogMapEmpty() {
				retryUpload(logs.GetFailedLogMap())
			}
			logs.PrintScoreBoard()
			logs.CloseAll()
		},
	}

	uploadCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	uploadCmd.MarkFlagRequired("profile") //nolint:errcheck
	uploadCmd.Flags().StringVar(&uploadPath, "upload-path", "", "The directory or file in which contains file(s) to be uploaded")
	uploadCmd.MarkFlagRequired("upload-path") //nolint:errcheck
	uploadCmd.Flags().BoolVar(&batch, "batch", false, "Upload in parallel")
	uploadCmd.Flags().IntVar(&numParallel, "numparallel", 3, "Number of uploads to run in parallel")
	uploadCmd.Flags().BoolVar(&includeSubDirName, "include-subdirname", true, "Include subdirectory names in file name")
	uploadCmd.Flags().BoolVar(&forceMultipart, "force-multipart", false, "Force to use multipart upload if possible")
	uploadCmd.Flags().BoolVar(&hasMetadata, "metadata", false, "Search for and upload file metadata alongside the file")
	uploadCmd.Flags().StringVar(&bucketName, "bucket", "", "The bucket to which files will be uploaded. If not provided, defaults to Gen3's configured DATA_UPLOAD_BUCKET.")
	RootCmd.AddCommand(uploadCmd)
}
