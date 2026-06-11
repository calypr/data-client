package cmd

import (
	"context"
	"fmt"

	"github.com/calypr/calypr-cli/g3client"
	"github.com/calypr/calypr-cli/logs"
	sydownload "github.com/calypr/syfon/client/transfer/download"
	"github.com/spf13/cobra"
)

func init() {
	var guid string
	var manifestPath string
	var downloadPath string
	var numParallel int

	var downloadCmd = &cobra.Command{
		Use:     "download",
		Aliases: []string{"download-single"},
		Short:   "Download file(s) from object storage.",
		Long: `Downloads files through the Syfon-backed download surface.

Use --guid for a single file or --manifest for multiple files.`,
		Example: "./calypr-cli download --profile=<profile-name> --guid=206dfaa6-bcf1-4bc9-b2d0-77179f0f48fc\n" +
			"./calypr-cli download --profile=<profile-name> --manifest=<path-to-manifest/manifest.json> --download-path=<path-to-file-dir/>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if guid == "" && manifestPath == "" {
				return fmt.Errorf("one of --guid or --manifest is required")
			}
			if guid != "" && manifestPath != "" {
				return fmt.Errorf("--guid and --manifest cannot be used together")
			}

			logger, logCloser := logs.New(profile, logs.WithConsole(), logs.WithFailedLog(), logs.WithSucceededLog(), logs.WithScoreboard())
			defer logCloser()

			g3I, err := g3client.NewGen3Interface(profile, logger)
			if err != nil {
				return fmt.Errorf("failed to parse config on profile %s: %w", profile, err)
			}

			syfon := g3I.SyfonClient()
			if syfon == nil {
				return fmt.Errorf("failed to initialize syfon client")
			}

			if manifestPath != "" {
				guids, err := loadManifestGuids(manifestPath)
				if err != nil {
					return fmt.Errorf("failed to read manifest %s: %w", manifestPath, err)
				}
				if err := sydownload.DownloadMultiple(context.Background(), syfon.DRS(), syfon.Data(), guids, downloadPath, numParallel, false); err != nil {
					return err
				}
				return nil
			}

			if err := sydownload.DownloadSingleWithProgress(context.Background(), syfon.DRS(), syfon.Data(), guid, downloadPath, "original"); err != nil {
				return err
			}
			return nil
		},
	}

	downloadCmd.Flags().StringVar(&guid, "guid", "", "Specify the guid for the data you would like to work with")
	downloadCmd.Flags().StringVar(&manifestPath, "manifest", "", "Manifest JSON for downloading multiple GUIDs")
	downloadCmd.Flags().StringVar(&downloadPath, "download-path", ".", "The directory in which to store the downloaded files")
	downloadCmd.Flags().IntVar(&numParallel, "numparallel", 1, "Number of downloads to run in parallel")
	RootCmd.AddCommand(downloadCmd)
}
