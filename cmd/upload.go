package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/calypr/calypr-cli/g3client"
	"github.com/calypr/calypr-cli/logs"
	syclient "github.com/calypr/syfon/client"
	sycommon "github.com/calypr/syfon/client/common"
	"github.com/spf13/cobra"
)

func init() {
	var bucketName string
	var includeSubDirName bool
	var uploadPath string
	var filePath string
	var multipartFilePath string
	var batch bool
	var numParallel int
	var hasMetadata bool
	var guid string
	var manifestPath string
	var failedLogPath string
	var multipart bool
	var uploadCmd = &cobra.Command{
		Use:   "upload",
		Short: "Upload file(s) to object storage.",
		Long: `Uploads files through the Syfon-backed upload surface.

This command also covers existing-GUID uploads, manifest uploads, forced
multipart uploads, and failed-log retries so that the CLI only needs one
top-level upload entrypoint.`,
		Example: "./calypr-cli upload --profile=<profile-name> --upload-path=<path-to-files/data.bam>\n" +
			"./calypr-cli upload --profile=<profile-name> --upload-path=<path-to-files/folder/>\n" +
			"./calypr-cli upload --profile=<profile-name> --guid=<existing-guid> --file=<path-to-file>\n" +
			"./calypr-cli upload --profile=<profile-name> --manifest=<path-to-manifest/manifest.json> --upload-path=<path-to-file-dir/>\n" +
			"./calypr-cli upload --profile=<profile-name> --file-path=<path-to-large-file> --multipart\n" +
			"./calypr-cli upload --profile=<profile-name> --failed-log-path=/path/to/failed_log.json",
		Run: func(cmd *cobra.Command, args []string) {
			resolvedUploadPath, err := resolveUploadInputPath(uploadPath, filePath, multipartFilePath)
			if err != nil {
				log.Fatalf("Invalid upload flags: %v", err)
			}

			ctx := context.Background()
			rootLogger, logCloser := logs.New(profile, logs.WithSucceededLog(), logs.WithScoreboard(), logs.WithFailedLog())
			defer logCloser()
			// Instantiate interface to Gen3
			g3i, err := g3client.NewGen3Interface(profile, rootLogger)
			if err != nil {
				log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
			}
			logger := g3i.Logger()
			syfon := g3i.SyfonClient()
			if syfon == nil {
				logger.Fatalf("failed to initialize syfon client")
			}

			uploadOptions := syclient.UploadOptions{
				Bucket:            bucketName,
				IncludeSubDirName: includeSubDirName,
				HasMetadata:       hasMetadata,
				Batch:             batch,
				NumParallel:       numParallel,
				GUID:              guid,
				ManifestPath:      manifestPath,
				ShowProgress:      guid != "" || manifestPath != "" || multipart,
				ForceMultipart:    multipart,
			}

			switch {
			case failedLogPath != "":
				if resolvedUploadPath != "" || manifestPath != "" || guid != "" {
					logger.Fatalf("--failed-log-path cannot be combined with file, guid, or manifest upload flags")
				}
				if _, err := sycommon.LoadFailedLog(failedLogPath); err != nil {
					logger.Fatalf("Cannot read failed log: %v", err)
				}
				err = syclient.Upload(ctx, syfon.Data(), "", syclient.UploadOptions{
					RetryFailedLogPath: failedLogPath,
				})
			default:
				if resolvedUploadPath == "" {
					logger.Fatalf("one of --upload-path, --file, --file-path, or --failed-log-path is required")
				}
				if manifestPath != "" && guid != "" {
					logger.Fatalf("--manifest cannot be combined with --guid")
				}
				if hasMetadata {
					hasShepherd, err := g3i.FenceClient().CheckForShepherdAPI(ctx)
					if err != nil {
						logger.Printf("WARNING: Error when checking for Shepherd API: %v", err)
					} else if !hasShepherd {
						logger.Fatalf("ERROR: Metadata upload (`--metadata`) is not supported in the environment you are uploading to. Double check that you are uploading to the right profile.")
					}
				}
				err = syclient.Upload(ctx, syfon.Data(), resolvedUploadPath, uploadOptions)
			}
			if err != nil {
				logger.Error("Upload failed", "error", err)
				return
			}
			g3i.Logger().Scoreboard().PrintSB()
		},
	}

	uploadCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	uploadCmd.MarkFlagRequired("profile") //nolint:errcheck
	uploadCmd.Flags().StringVar(&uploadPath, "upload-path", "", "The directory or file in which contains file(s) to be uploaded")
	uploadCmd.Flags().StringVar(&filePath, "file", "", "Deprecated alias for --upload-path when uploading a single file")
	uploadCmd.Flags().StringVar(&multipartFilePath, "file-path", "", "Deprecated alias for --upload-path, used by legacy multipart uploads")
	uploadCmd.Flags().StringVar(&guid, "guid", "", "Upload to an existing GUID instead of creating a new one")
	uploadCmd.Flags().StringVar(&manifestPath, "manifest", "", "Manifest JSON for uploads to pre-existing GUIDs")
	uploadCmd.Flags().StringVar(&failedLogPath, "failed-log-path", "", "Retry uploads listed in a failed_log.json file")
	uploadCmd.Flags().BoolVar(&multipart, "multipart", false, "Force managed multipart upload")
	uploadCmd.Flags().BoolVar(&batch, "batch", false, "Upload in parallel")
	uploadCmd.Flags().IntVar(&numParallel, "numparallel", 3, "Number of uploads to run in parallel")
	uploadCmd.Flags().BoolVar(&includeSubDirName, "include-subdirname", true, "Include subdirectory names in file name")
	uploadCmd.Flags().BoolVar(&hasMetadata, "metadata", false, "Search for and upload file metadata alongside the file")
	uploadCmd.Flags().StringVar(&bucketName, "bucket", "", "The bucket to which files will be uploaded. If not provided, defaults to Gen3's configured DATA_UPLOAD_BUCKET.")
	RootCmd.AddCommand(uploadCmd)
}

func resolveUploadInputPath(primaryPath, filePath, multipartPath string) (string, error) {
	var resolved string
	for _, candidate := range []string{primaryPath, filePath, multipartPath} {
		if candidate == "" {
			continue
		}
		if resolved == "" {
			resolved = candidate
			continue
		}
		if resolved != candidate {
			return "", fmt.Errorf("conflicting upload path flags: %q and %q", resolved, candidate)
		}
	}
	return resolved, nil
}
