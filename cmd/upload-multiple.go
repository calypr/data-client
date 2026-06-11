package cmd

// Deprecated: Use "upload" instead for new uploads (without pre-existing GUIDs).
import (
	"context"
	"fmt"

	"github.com/calypr/calypr-cli/g3client"
	"github.com/calypr/calypr-cli/logs"
	syclient "github.com/calypr/syfon/client"
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
		Hidden: true,
		Use:    "upload-multiple",
		Short:  "Upload multiple files from a specified manifest (uses pre-existing GUIDs)",
		Long: `Get presigned URLs for multiple files specified in a manifest file and then upload all of them.
This command is for uploading to existing GUIDs (e.g., from a downloaded manifest).
For new uploads (new GUIDs generated), use "calypr-cli upload" instead.

Options to run multipart uploads for large files and parallel batch uploading are available.`,
		Example: `./calypr-cli upload-multiple --profile=<profile-name> --manifest=<path-to-manifest/manifest.json> --upload-path=<path-to-file-dir/> --bucket=<bucket-name> --batch`,
		Run: func(cmd *cobra.Command, args []string) {
			// Warning message
			fmt.Printf("Notice: this command uploads to pre-existing GUIDs from a manifest.\nIf you want to upload new files (new GUIDs generated automatically), use \"./calypr-cli upload\" instead.\n\n")

			ctx := context.Background()
			logger, closer := logs.New(profile, logs.WithSucceededLog(), logs.WithFailedLog(), logs.WithScoreboard())
			defer closer()

			g3i, err := g3client.NewGen3Interface(profile, logger)
			if err != nil {
				logger.Fatalf("Failed to parse config on profile %s: %v", profile, err)
			}

			syfon := g3i.SyfonClient()
			if syfon == nil {
				logger.Fatal("failed to initialize syfon client")
			}
			err = syclient.Upload(ctx, syfon.Data(), uploadPath, syclient.UploadOptions{
				Bucket:            bucketName,
				IncludeSubDirName: includeSubDirName,
				Batch:             batch,
				NumParallel:       numParallel,
				ManifestPath:      manifestPath,
				ShowProgress:      true,
			})
			if err != nil {
				logger.Println("Upload failed:", err)
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
