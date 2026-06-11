package cmd

// Deprecated: Use upload instead.
import (
	"context"
	"log"

	"github.com/calypr/calypr-cli/g3client"
	"github.com/calypr/calypr-cli/logs"
	syclient "github.com/calypr/syfon/client"
	"github.com/spf13/cobra"
)

func init() {
	var guid string
	var filePath string
	var bucketName string

	var uploadSingleCmd = &cobra.Command{
		Hidden:  true,
		Use:     "upload-single",
		Short:   "Upload a single file to a GUID",
		Long:    `Gets a presigned URL for which to upload a file associated with a GUID and then uploads the specified file.`,
		Example: `./calypr-cli upload-single --profile=<profile-name> --guid=f6923cf3-xxxx-xxxx-xxxx-14ab3f84f9d6 --file=<path-to-file>`,
		Run: func(cmd *cobra.Command, args []string) {
			logger, closer := logs.New(profile, logs.WithSucceededLog(), logs.WithFailedLog(), logs.WithScoreboard(), logs.WithConsole())
			defer closer()

			g3i, err := g3client.NewGen3Interface(profile, logger)
			if err != nil {
				log.Fatalf("Failed to parse config on profile %s: %v", profile, err)
			}
			syfon := g3i.SyfonClient()
			if syfon == nil {
				log.Fatal("failed to initialize syfon client")
			}
			err = syclient.Upload(context.Background(), syfon.Data(), filePath, syclient.UploadOptions{
				Bucket:       bucketName,
				GUID:         guid,
				ShowProgress: true,
			})
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
