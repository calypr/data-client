package cmd

import (
	"context"
	"log"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	sydrs "github.com/calypr/syfon/client/drs"
	sylogs "github.com/calypr/syfon/client/pkg/logs"
	syrequest "github.com/calypr/syfon/client/pkg/request"
	sydownload "github.com/calypr/syfon/client/xfer/download"
	"github.com/spf13/cobra"
)

func init() {
	var guid string
	var downloadPath string
	var profile string

	var downloadSingleCmd = &cobra.Command{
		Use:     "download-single",
		Short:   "Download a single file from a GUID",
		Long:    `Gets a presigned URL for a file from a GUID and then downloads the specified file.`,
		Example: `./data-client download-single --profile=<profile-name> --guid=206dfaa6-bcf1-4bc9-b2d0-77179f0f48fc`,
		Run: func(cmd *cobra.Command, args []string) {
			logger, logCloser := logs.New(profile, logs.WithConsole(), logs.WithFailedLog(), logs.WithSucceededLog(), logs.WithScoreboard())
			defer logCloser()

			var dc sydrs.Client
			if backendType == "drs" {
				config := conf.NewConfigure(logger.Logger)
				cred, err := config.Load(profile)
				if err != nil {
					log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
				}
				req := syrequest.NewRequestInterface(
					sylogs.NewGen3Logger(logger.Logger, "", ""),
					cred,
					config,
				)
				dc = sydrs.NewLocalDrsClient(req, cred.APIEndpoint, sylogs.NewGen3Logger(logger.Logger, "", ""))
			} else {
				g3I, err := g3client.NewGen3Interface(profile, logger)
				if err != nil {
					log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
				}
				dc = g3I.DRSClient()
			}

			objects := []common.ManifestObject{
				{
					GUID: guid,
				},
			}
			err := sydownload.DownloadMultiple(
				context.Background(),
				dc,
				dc,
				objects,
				downloadPath,
				"original",
				true,
				false,
				"",
				1,
				false,
			)
			if err != nil {
				logger.Println(err.Error())
			}
		},
	}

	downloadSingleCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	downloadSingleCmd.MarkFlagRequired("profile") //nolint:errcheck
	downloadSingleCmd.Flags().StringVar(&guid, "guid", "", "Specify the guid for the data you would like to work with")
	downloadSingleCmd.MarkFlagRequired("guid") //nolint:errcheck
	downloadSingleCmd.Flags().StringVar(&downloadPath, "download-path", ".", "The directory in which to store the downloaded files")
	RootCmd.AddCommand(downloadSingleCmd)
}
