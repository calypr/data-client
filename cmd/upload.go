package cmd

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/upload"
	"github.com/spf13/cobra"
)

func init() {
	var bucketName string
	var includeSubDirName bool
	var uploadPath string
	var batch bool
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

			ctx := context.Background()
			Logger, logCloser := logs.New(profile, logs.WithSucceededLog(), logs.WithScoreboard(), logs.WithFailedLog())
			defer logCloser()
			// Instantiate interface to Gen3
			g3i, err := g3client.NewGen3Interface(profile, Logger)
			if err != nil {
				log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
			}

			logger := g3i.Logger()
			if hasMetadata {
				hasShepherd, err := g3i.Fence().CheckForShepherdAPI(ctx)
				if err != nil {
					logger.Printf("WARNING: Error when checking for Shepherd API: %v", err)
				} else {
					if !hasShepherd {
						logger.Fatalf("ERROR: Metadata upload (`--metadata`) is not supported in the environment you are uploading to. Double check that you are uploading to the right profile.")
					}
				}
			}

			uploadPath, _ = common.GetAbsolutePath(uploadPath)
			filePaths, err := common.ParseFilePaths(uploadPath, hasMetadata)
			if err != nil {
				logger.Fatalf("Error when parsing file paths: %s", err.Error())
			}
			uploadRequestObjects := make([]common.FileUploadRequestObject, 0, len(filePaths))

			logger.Println("\nThe following file(s) has been found in path \"" + uploadPath + "\" and will be uploaded:")
			for _, filePath := range filePaths {
				// Use ProcessFilename to create the unified object (GUID is empty here, as this command requests a new GUID)
				// ProcessFilename signature: (uploadPath, filePath, objectId, includeSubDirName, includeMetadata)
				furObject, err := upload.ProcessFilename(g3i.Logger(), uploadPath, filePath, "", includeSubDirName, hasMetadata)
				furObject.Bucket = bucketName

				// Handle case where ProcessFilename fails (e.g., metadata parsing error)
				if err != nil {
					// Use the data available for logging the failure
					g3i.Logger().Failed(filePath, filepath.Base(filePath), common.FileMetadata{}, "", 0, false)
					logger.Println("Error processing file path or metadata: " + err.Error())
					continue
				}

				// Optional: Display file path before proceeding
				file, _ := os.Open(filePath)
				if fi, _ := file.Stat(); !fi.IsDir() {
					logger.Println("\t" + filePath)
				}
				file.Close()

				uploadRequestObjects = append(uploadRequestObjects, furObject)
			}
			// fmt.Fprintln(os.Stderr)
			logger.Println()

			if len(uploadRequestObjects) == 0 {
				logger.Println("No valid file upload requests were created.")
				return
			}

			singlePartObjects, multipartObjects := upload.SeparateSingleAndMultipartUploads(g3i, uploadRequestObjects)

			if batch {
				workers, respCh, errCh, batchFURObjects := upload.InitBatchUploadChannels(numParallel, len(singlePartObjects))

				for _, furObject := range singlePartObjects {
					if len(batchFURObjects) < workers {
						batchFURObjects = append(batchFURObjects, furObject)
					} else {
						upload.BatchUpload(ctx, g3i, batchFURObjects, workers, respCh, errCh, bucketName)
						batchFURObjects = []common.FileUploadRequestObject{furObject}
					}
				}
				if len(batchFURObjects) > 0 {
					upload.BatchUpload(ctx, g3i, batchFURObjects, workers, respCh, errCh, bucketName)
				}

				if len(errCh) > 0 {
					close(errCh)
					for err := range errCh {
						if err != nil {
							logger.Printf("Error occurred during uploading: %s\n", err.Error())
						}
					}
				}
			} else {
				for _, furObject := range singlePartObjects {
					file, err := os.Open(furObject.SourcePath)
					if err != nil {
						logger.Failed(furObject.SourcePath, furObject.ObjectKey, furObject.FileMetadata, furObject.GUID, 0, false)
						logger.Println("File open error: " + err.Error())
						continue
					}
					defer file.Close()
					fi, err := file.Stat()
					if err != nil {
						logger.Failed(furObject.SourcePath, furObject.ObjectKey, furObject.FileMetadata, furObject.GUID, 0, false)
						logger.Println("File stat error for file" + fi.Name() + ", file may be missing or unreadable because of permissions.\n")
						continue
					}
					upload.UploadSingle(ctx, g3i, furObject, true)
				}
			}

			if len(multipartObjects) > 0 {
				cred := g3i.GetCredential()
				if cred.UseShepherd == "true" ||
					cred.UseShepherd == "" && common.DefaultUseShepherd == true {
					logger.Printf("error: Shepherd currently does not support multipart uploads. For the moment, please disable Shepherd with\n    $ data-client configure --profile=%v --use-shepherd=false\nand try again", cred.Profile)
					return
				}
				g3i.Logger().Println("Multipart uploading...")
				for _, furObject := range multipartObjects {
					file, err := os.Open(furObject.SourcePath)
					if err != nil {
						logger.Failed(furObject.SourcePath, furObject.ObjectKey, furObject.FileMetadata, furObject.GUID, 0, false)
						logger.Println("File open error: " + err.Error())
						continue
					}
					err = upload.MultipartUpload(ctx, g3i, furObject, file, true)
					if err != nil {
						g3i.Logger().Println(err.Error())
					} else {
						g3i.Logger().Scoreboard().IncrementSB(0)
					}
				}
			}
			if len(g3i.Logger().GetSucceededLogMap()) == 0 {
				upload.RetryFailedUploads(ctx, g3i, g3i.Logger().GetFailedLogMap())
			}
			g3i.Logger().Scoreboard().PrintSB()
		},
	}

	uploadCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	uploadCmd.MarkFlagRequired("profile") //nolint:errcheck
	uploadCmd.Flags().StringVar(&uploadPath, "upload-path", "", "The directory or file in which contains file(s) to be uploaded")
	uploadCmd.MarkFlagRequired("upload-path") //nolint:errcheck
	uploadCmd.Flags().BoolVar(&batch, "batch", false, "Upload in parallel")
	uploadCmd.Flags().IntVar(&numParallel, "numparallel", 3, "Number of uploads to run in parallel")
	uploadCmd.Flags().BoolVar(&includeSubDirName, "include-subdirname", true, "Include subdirectory names in file name")
	uploadCmd.Flags().BoolVar(&hasMetadata, "metadata", false, "Search for and upload file metadata alongside the file")
	uploadCmd.Flags().StringVar(&bucketName, "bucket", "", "The bucket to which files will be uploaded. If not provided, defaults to Gen3's configured DATA_UPLOAD_BUCKET.")
	RootCmd.AddCommand(uploadCmd)
}
