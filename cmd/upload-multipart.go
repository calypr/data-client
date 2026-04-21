package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	sytransfer "github.com/calypr/syfon/client/transfer"
	syupload "github.com/calypr/syfon/client/xfer/upload"
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
		Short: "Upload a single file using managed multipart upload",
		Long: `Uploads a file to object storage using managed multipart upload
(init -> presigned part URLs -> complete).`,
		Example: `./data-client upload-multipart --profile=myprofile --file-path=./large.bam
./data-client upload-multipart --profile=myprofile --file-path=./data.bam --guid=existing-guid`,
		Run: func(cmd *cobra.Command, args []string) {
			// Initialize logger
			logger, logCloser := logs.New(profile, logs.WithConsole())
			defer logCloser()

			logger, closer := logs.New(profile, logs.WithSucceededLog(), logs.WithFailedLog(), logs.WithScoreboard())
			defer closer()

			g3, err := g3client.NewGen3Interface(
				profile,
				logger,
			)

			if err != nil {
				logger.Fatalf("failed to initialize Gen3 interface: %v", err)
			}
			bk := g3.DRSClient()
			uploader, ok := bk.(sytransfer.Uploader)
			if !ok {
				logger.Fatal("DRS client does not implement transfer.Uploader")
			}

			absPath, err := common.GetAbsolutePath(filePath)
			if err != nil {
				logger.Fatalf("invalid file path: %v", err)
			}

			fileInfo := common.FileUploadRequestObject{
				SourcePath:   absPath,
				ObjectKey:    filepath.Base(absPath),
				GUID:         guid,
				FileMetadata: common.FileMetadata{},
			}

			if fileInfo.Bucket == "" {
				fileInfo.Bucket = bucketName
			}
			if fileInfo.Bucket == "" {
				fileInfo.Bucket = bk.GetBucketName()
			}

			// Force multipart path by using direct multipart entrypoint.
			file, err := os.Open(fileInfo.SourcePath)
			if err != nil {
				logger.Fatal(err)
			}
			defer file.Close()
			err = syupload.MultipartUpload(context.Background(), uploader, fileInfo, file, true)
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
