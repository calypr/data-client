package cmd

import (
	"context"
	"log"

	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	syclient "github.com/calypr/syfon/client"
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
			rootLogger, logCloser := logs.New(profile, logs.WithSucceededLog(), logs.WithScoreboard(), logs.WithFailedLog())
			defer logCloser()
			// Instantiate interface to Gen3
			g3i, err := g3client.NewGen3Interface(profile, rootLogger)
			if err != nil {
				log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
			}
			logger := g3i.Logger()
			if hasMetadata {
				hasShepherd, err := g3i.FenceClient().CheckForShepherdAPI(ctx)
				if err != nil {
					logger.Printf("WARNING: Error when checking for Shepherd API: %v", err)
				} else {
					if !hasShepherd {
						logger.Fatalf("ERROR: Metadata upload (`--metadata`) is not supported in the environment you are uploading to. Double check that you are uploading to the right profile.")
					}
				}
			}

			syfon := g3i.SyfonClient()
			if syfon == nil {
				logger.Fatalf("failed to initialize syfon client")
			}

			err = syclient.Upload(ctx, syfon.Data(), uploadPath, syclient.UploadOptions{
				Bucket:            bucketName,
				IncludeSubDirName: includeSubDirName,
				HasMetadata:       hasMetadata,
				Batch:             batch,
				NumParallel:       numParallel,
			})
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
	uploadCmd.MarkFlagRequired("upload-path") //nolint:errcheck
	uploadCmd.Flags().BoolVar(&batch, "batch", false, "Upload in parallel")
	uploadCmd.Flags().IntVar(&numParallel, "numparallel", 3, "Number of uploads to run in parallel")
	uploadCmd.Flags().BoolVar(&includeSubDirName, "include-subdirname", true, "Include subdirectory names in file name")
	uploadCmd.Flags().BoolVar(&hasMetadata, "metadata", false, "Search for and upload file metadata alongside the file")
	uploadCmd.Flags().StringVar(&bucketName, "bucket", "", "The bucket to which files will be uploaded. If not provided, defaults to Gen3's configured DATA_UPLOAD_BUCKET.")
	RootCmd.AddCommand(uploadCmd)
}
