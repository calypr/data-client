package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
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

			logger, closer := logs.New(profile, logs.WithSucceededLog(), logs.WithFailedLog(), logs.WithScoreboard())
			defer closer()

			g3, err := client.NewGen3Interface(
				profile,
				logger,
			)

			if err != nil {
				logger.Fatalf("failed to initialize Gen3 interface: %w", err)
			}

			absPath, err := common.GetAbsolutePath(filePath)
			if err != nil {
				logger.Fatalf("invalid file path: %w", err)
			}

			fileInfo := common.FileUploadRequestObject{
				FilePath:     absPath,
				Filename:     filepath.Base(absPath),
				GUID:         guid,
				FileMetadata: common.FileMetadata{},
			}

			file, err := os.Open(absPath)
			if err != nil {
				logger.Fatalf("cannot open file %s: %w", absPath, err)
			}
			defer file.Close()

			err = upload.MultipartUpload(context.Background(), g3, fileInfo, file, true)
			if err != nil {
				logger.Fatal(err)
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
