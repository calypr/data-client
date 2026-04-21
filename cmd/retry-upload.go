package cmd

import (
	"context"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	sylogs "github.com/calypr/syfon/client/pkg/logs"
	sytransfer "github.com/calypr/syfon/client/transfer"
	syupload "github.com/calypr/syfon/client/xfer/upload"

	"github.com/spf13/cobra"
)

func init() {
	var failedLogPath, profile string

	var retryUploadCmd = &cobra.Command{
		Use:     "retry-upload",
		Short:   "Retry failed uploads from a failed_log.json",
		Long:    `Re-uploads files listed in a failed log using exponential backoff and progress bars.`,
		Example: `./data-client retry-upload --profile=myprofile --failed-log-path=/path/to/failed_log.json`,
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
			bk := g3.DRSClient()
			uploader, ok := bk.(sytransfer.Uploader)
			if !ok {
				Logger.Fatalf("DRS client does not implement transfer.Uploader")
			}

			logger := g3.Logger()

			// Create scoreboard with our logger injected
			sb := logs.NewSB(common.MaxRetryCount, logger.Logger)

			// Load failed log
			failedMap, err := common.LoadFailedLog(failedLogPath)
			if err != nil {
				logger.Fatalf("Cannot read failed log: %v", err)
			}

			// Unified DRS Client serves as both logical resolver and technical movement writer Across S3, GCS, and Azure.
			syupload.RetryFailedUploads(context.Background(), uploader, sylogs.NewGen3Logger(Logger.Logger, "", ""), failedMap)
			sb.PrintSB()
		},
	}

	retryUploadCmd.Flags().StringVar(&profile, "profile", "", "Profile to use")
	retryUploadCmd.MarkFlagRequired("profile")

	retryUploadCmd.Flags().StringVar(&failedLogPath, "failed-log-path", "", "Path to failed_log.json")
	retryUploadCmd.MarkFlagRequired("failed-log-path")

	RootCmd.AddCommand(retryUploadCmd)
}
