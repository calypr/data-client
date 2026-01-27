package cmd

import (
	"context"

	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/upload"

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

			g3, err := client.NewGen3Interface(profile, Logger)
			if err != nil {
				Logger.Fatalf("Failed to initialize client: %v", err)
			}

			logger := g3.Logger()

			// Create scoreboard with our logger injected
			sb := logs.NewSB(common.MaxRetryCount, logger)

			// Load failed log
			failedMap, err := common.LoadFailedLog(failedLogPath)
			if err != nil {
				logger.Fatalf("Cannot read failed log: %v", err)
			}

			upload.RetryFailedUploads(context.Background(), g3, failedMap)
			sb.PrintSB()
		},
	}

	retryUploadCmd.Flags().StringVar(&profile, "profile", "", "Profile to use")
	retryUploadCmd.MarkFlagRequired("profile")

	retryUploadCmd.Flags().StringVar(&failedLogPath, "failed-log-path", "", "Path to failed_log.json")
	retryUploadCmd.MarkFlagRequired("failed-log-path")

	RootCmd.AddCommand(retryUploadCmd)
}
