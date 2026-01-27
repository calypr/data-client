package cmd

// Deprecated: Use "upload" instead for new uploads (without pre-existing GUIDs).
import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/calypr/data-client/common"
	client "github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/upload"
	"github.com/spf13/cobra"
)

func init() {
	var bucketName string
	var manifestPath string
	var uploadPath string
	var batch bool
	var numParallel int
	var includeSubDirName bool

	uploadMultipleCmd := &cobra.Command{
		Use:   "upload-multiple",
		Short: "Upload multiple files from a specified manifest (uses pre-existing GUIDs)",
		Long: `Get presigned URLs for multiple files specified in a manifest file and then upload all of them.
This command is for uploading to existing GUIDs (e.g., from a downloaded manifest).
For new uploads (new GUIDs generated), use "data-client upload" instead.

Options to run multipart uploads for large files and parallel batch uploading are available.`,
		Example: `./data-client upload-multiple --profile=<profile-name> --manifest=<path-to-manifest/manifest.json> --upload-path=<path-to-file-dir/> --bucket=<bucket-name> --batch`,
		Run: func(cmd *cobra.Command, args []string) {
			// Warning message
			fmt.Printf("Notice: this command uploads to pre-existing GUIDs from a manifest.\nIf you want to upload new files (new GUIDs generated automatically), use \"./data-client upload\" instead.\n\n")

			ctx := context.Background()
			noopProgress := func(common.ProgressEvent) error { return nil }

			logger, closer := logs.New(profile, logs.WithSucceededLog(), logs.WithFailedLog(), logs.WithScoreboard())
			defer closer()

			g3i, err := client.NewGen3Interface(profile, logger)
			if err != nil {
				logger.Fatalf("Failed to parse config on profile %s: %v", profile, err)
			}

			// Basic config validation
			profileConfig := g3i.GetCredential()
			if profileConfig.APIEndpoint == "" {
				logger.Fatal("No APIEndpoint found in configuration. Run \"./data-client configure\" first.")
			}
			host, err := url.Parse(profileConfig.APIEndpoint)
			if err != nil {
				logger.Fatal("Error parsing APIEndpoint:", err)
			}
			dataExplorerURL := host.Scheme + "://" + host.Host + "/explorer"

			// Load manifest
			var objects []common.ManifestObject
			manifestBytes, err := os.ReadFile(manifestPath)
			if err != nil {
				logger.Fatalf("Failed reading manifest %s: %v\nA valid manifest can be acquired from %s", manifestPath, err, dataExplorerURL)
			}
			if err := json.Unmarshal(manifestBytes, &objects); err != nil {
				logger.Fatalf("Invalid manifest JSON: %v", err)
			}

			absUploadPath, err := common.GetAbsolutePath(uploadPath)
			if err != nil {
				logger.Fatalf("Error resolving upload path: %v", err)
			}

			// Build FileUploadRequestObjects using existing GUIDs
			var requests []common.FileUploadRequestObject
			logger.Println("\nProcessing manifest entries...")

			for _, obj := range objects {
				localFilePath := filepath.Join(absUploadPath, obj.Title)

				fur, err := upload.ProcessFilename(logger, absUploadPath, localFilePath, obj.ObjectID, includeSubDirName, false)
				if err != nil {
					logger.Printf("Skipping %s: %v\n", localFilePath, err)
					logger.Failed(localFilePath, filepath.Base(localFilePath), common.FileMetadata{}, obj.ObjectID, 0, false)
					continue
				}

				// GUID comes from manifest → override
				fur.GUID = obj.ObjectID
				fur.Bucket = bucketName
				fur.Progress = noopProgress

				logger.Println("\t" + localFilePath + " → GUID " + obj.ObjectID)
				requests = append(requests, fur)
			}

			if len(requests) == 0 {
				logger.Println("No valid files found to upload from manifest.")
				return
			}

			// Classify single vs multipart
			single, multi := upload.SeparateSingleAndMultipartUploads(g3i, requests)

			// Upload single-part files
			if batch {
				workers, respCh, errCh, batchFURObjects := upload.InitBatchUploadChannels(numParallel, len(single))
				for i, furObject := range single {
					// FileInfo processing and path normalization are already done, so we use the object directly
					if len(batchFURObjects) < workers {
						batchFURObjects = append(batchFURObjects, furObject)
					} else {
						upload.BatchUpload(ctx, g3i, batchFURObjects, workers, respCh, errCh, bucketName)
						batchFURObjects = []common.FileUploadRequestObject{furObject}
					}
					if i == len(single)-1 && len(batchFURObjects) > 0 {
						upload.BatchUpload(ctx, g3i, batchFURObjects, workers, respCh, errCh, bucketName)
					}
				}
			} else {
				for _, req := range single {
					upload.UploadSingle(ctx, profileConfig.Profile, req.GUID, req.GUID, req.FilePath, req.Bucket, true, nil)
				}
			}

			// Upload multipart files
			for _, req := range multi {

				file, err := os.Open(req.FilePath)
				if err != nil {
					g3i.Logger().Printf("Error opening file %s : %v", req.FilePath, err)
					continue
				}

				err = upload.MultipartUpload(ctx, g3i, req, file, true)
				if err != nil {
					logger.Println("Multipart upload failed:", err)
				}
			}

			// Retry logic (only if nothing succeeded initially)
			if len(logger.GetSucceededLogMap()) == 0 {
				failed := logger.GetFailedLogMap()
				if len(failed) > 0 {
					upload.RetryFailedUploads(ctx, g3i, failed)
				}
			}

			logger.Scoreboard().PrintSB()
		},
	}

	// Flags
	uploadMultipleCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	uploadMultipleCmd.MarkFlagRequired("profile")

	uploadMultipleCmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to the manifest JSON file")
	uploadMultipleCmd.MarkFlagRequired("manifest")

	uploadMultipleCmd.Flags().StringVar(&uploadPath, "upload-path", "", "Directory containing the files to upload")
	uploadMultipleCmd.MarkFlagRequired("upload-path")

	uploadMultipleCmd.Flags().BoolVar(&batch, "batch", true, "Upload single-part files in parallel")
	uploadMultipleCmd.Flags().IntVar(&numParallel, "numparallel", 4, "Number of parallel uploads")

	uploadMultipleCmd.Flags().StringVar(&bucketName, "bucket", "", "Target bucket (defaults to configured DATA_UPLOAD_BUCKET)")

	uploadMultipleCmd.Flags().BoolVar(&includeSubDirName, "include-subdirname", true, "Include subdirectory names in object key")

	RootCmd.AddCommand(uploadMultipleCmd)
}
