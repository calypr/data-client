package cmd

import (
	"context"

	"github.com/calypr/calypr-cli/g3client"
	"github.com/calypr/calypr-cli/logs"
	syclient "github.com/calypr/syfon/client"
	sycommon "github.com/calypr/syfon/client/common"

	"github.com/spf13/cobra"
)

func init() {
	var failedLogPath, profile string

	var retryUploadCmd = &cobra.Command{
		Hidden:  true,
		Use:     "retry-upload",
		Short:   "Retry failed uploads from a failed_log.json",
		Long:    `Re-uploads files listed in a failed log using exponential backoff and progress bars.`,
		Example: `./calypr-cli retry-upload --profile=myprofile --failed-log-path=/path/to/failed_log.json`,
		Run: func(cmd *cobra.Command, args []string) {
			Logger, closer := logs.New(profile,
				logs.WithConsole(),
				logs.WithMessageFile(),
				logs.WithFailedLog(),
				logs.WithSucceededLog(),
			)
			defer closer()

			g3, err := g3client.NewGen3Interface(profile, Logger)
			if err != nil {
				Logger.Fatalf("Failed to initialize client: %v", err)
			}
			if _, err := sycommon.LoadFailedLog(failedLogPath); err != nil {
				Logger.Fatalf("Cannot read failed log: %v", err)
			}
			syfon := g3.SyfonClient()
			if syfon == nil {
				Logger.Fatal("failed to initialize syfon client")
			}
			err = syclient.Upload(context.Background(), syfon.Data(), "", syclient.UploadOptions{
				RetryFailedLogPath: failedLogPath,
			})
			if err != nil {
				Logger.Fatalf("Retry upload failed: %v", err)
			}
		},
	}

	retryUploadCmd.Flags().StringVar(&profile, "profile", "", "Profile to use")
	retryUploadCmd.MarkFlagRequired("profile")

	retryUploadCmd.Flags().StringVar(&failedLogPath, "failed-log-path", "", "Path to failed_log.json")
	retryUploadCmd.MarkFlagRequired("failed-log-path")

	RootCmd.AddCommand(retryUploadCmd)
}
