package cmd

import (
	"context"
	"log"

	"github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/logs"
	"github.com/calypr/data-client/client/upload"
	"github.com/spf13/cobra"
)

func init() {
	var (
		profile    string
		filePath   string
		guid       string
		bucketName string
	)

	var uploadMultipartCmd = &cobra.Command{
		Use:   "upload-multipart",
		Short: "Upload a single file using multipart upload",
		Long: `Uploads a large file to object storage using multipart upload.
This method is resilient to network interruptions and supports resume capability.`,
		Example: `./data-client upload-multipart --profile=myprofile --file-path=./large.bam
./data-client upload-multipart --profile=myprofile --file-path=./data.bam --guid=existing-guid`,
		Run: func(cmd *cobra.Command, args []string) {
			// Initialize logger
			logger, logCloser := logs.New(profile, logs.WithConsole())
			defer logCloser()

			// Initialize Gen3 Interface
			g3i, err := client.NewGen3Interface(profile, logger)
			if err != nil {
				log.Fatalf("Fatal NewGen3Interface error: %s\n", err)
			}

			// Execute the upload
			// Note: We use the profile string directly as per the original wrapper signature
			err = upload.UploadSingleFileWrapper(context.Background(), profile, bucketName, filePath, guid, true)
			if err != nil {
				g3i.Logger().Fatalf("Upload failed: %s\n", err)
			}
		},
	}

	uploadMultipartCmd.Flags().StringVar(&profile, "profile", "", "Specify the profile to use for upload")
	uploadMultipartCmd.Flags().StringVar(&filePath, "file-path", "", "Path to the file to upload")
	uploadMultipartCmd.Flags().StringVar(&guid, "guid", "", "Optional existing GUID (otherwise generated)")
	uploadMultipartCmd.Flags().StringVar(&bucketName, "bucket", "", "Target bucket (defaults to configured DATA_UPLOAD_BUCKET)")

	_ = uploadMultipartCmd.MarkFlagRequired("profile")
	_ = uploadMultipartCmd.MarkFlagRequired("file-path")

	RootCmd.AddCommand(uploadMultipartCmd)
}
