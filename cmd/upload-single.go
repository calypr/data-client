package cmd

// Deprecated: Use upload instead.
import (
	"context"
	"log"
	"path/filepath"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/upload"
	"github.com/spf13/cobra"
)

func init() {
	var guid string
	var filePath string
	var bucketName string

	var uploadSingleCmd = &cobra.Command{
		Use:     "upload-single",
		Short:   "Upload a single file to a GUID",
		Long:    `Gets a presigned URL for which to upload a file associated with a GUID and then uploads the specified file.`,
		Example: `./data-client upload-single --profile=<profile-name> --guid=f6923cf3-xxxx-xxxx-xxxx-14ab3f84f9d6 --file=<path-to-file>`,
		Run: func(cmd *cobra.Command, args []string) {
			req := common.FileUploadRequestObject{
				SourcePath: filePath,
				ObjectKey:  filepath.Base(filePath),
				Bucket:     bucketName,
				GUID:       guid,
				Progress:   nil,
			}
			err := upload.UploadSingle(context.Background(), profile, req, true)
			if err != nil {
				log.Fatalln(err.Error())
			}
		},
	}
	uploadSingleCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	uploadSingleCmd.MarkFlagRequired("profile") //nolint:errcheck
	uploadSingleCmd.Flags().StringVar(&guid, "guid", "", "Specify the guid for the data you would like to work with")
	uploadSingleCmd.MarkFlagRequired("guid") //nolint:errcheck
	uploadSingleCmd.Flags().StringVar(&filePath, "file", "", "Specify file to upload to with --file=~/path/to/file")
	uploadSingleCmd.MarkFlagRequired("file") //nolint:errcheck
	uploadSingleCmd.Flags().StringVar(&bucketName, "bucket", "", "The bucket to which files will be uploaded. If not provided, defaults to Gen3's configured DATA_UPLOAD_BUCKET.")
	RootCmd.AddCommand(uploadSingleCmd)
}
