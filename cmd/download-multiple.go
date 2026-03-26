package cmd

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/download"
	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/localclient"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/transfer"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/spf13/cobra"
)

func init() {
	var manifestPath string
	var downloadPath string
	var filenameFormat string
	var rename bool
	var noPrompt bool
	var protocol string
	var numParallel int
	var skipCompleted bool

	var downloadMultipleCmd = &cobra.Command{
		Use:     "download-multiple",
		Short:   "Download multiple of files from a specified manifest",
		Long:    `Get presigned URLs for multiple of files specified in a manifest file and then download all of them.`,
		Example: `./data-client download-multiple --profile <profile-name> --manifest <path-to-manifest/manifest.json> --download-path <path-to-file-dir/>`,
		Run: func(cmd *cobra.Command, args []string) {
			// don't initialize transmission logs for non-uploading related commands

			logger, logCloser := logs.New(profile, logs.WithConsole(), logs.WithFailedLog(), logs.WithScoreboard(), logs.WithSucceededLog())
			defer logCloser()

			var dc drs.Client
			var bk transfer.Backend
			if backendType == "drs" {
				lc, err := localclient.NewLocalInterface(profile, logger)
				if err != nil {
					log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
				}
				dc = lc.DRSClient()
				bk = lc.DRSClient()
			} else {
				g3i, err := g3client.NewGen3Interface(profile, logger)
				if err != nil {
					log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
				}
				dc = g3i.DRSClient()
				bk = g3i.DRSClient()
			}

			manifestPath, _ = common.GetAbsolutePath(manifestPath)
			manifestFile, err := os.Open(manifestPath)
			if err != nil {
				logger.Fatalf("Failed to open manifest file %s, %v\n", manifestPath, err)
			}
			defer manifestFile.Close()
			manifestFileStat, err := manifestFile.Stat()
			if err != nil {
				logger.Fatalf("Failed to get manifest file stats %s, %v\n", manifestPath, err)
			}
			logger.Println("Reading manifest...")
			manifestFileSize := manifestFileStat.Size()
			manifestProgress := mpb.New(mpb.WithOutput(os.Stdout))
			manifestFileBar := manifestProgress.AddBar(manifestFileSize,
				mpb.PrependDecorators(
					decor.Name("Manifest "),
					decor.CountersKibiByte("% .1f / % .1f"),
				),
				mpb.AppendDecorators(decor.Percentage()),
			)

			manifestFileReader := manifestFileBar.ProxyReader(manifestFile)

			manifestBytes, err := io.ReadAll(manifestFileReader)
			if err != nil {
				logger.Fatalf("Failed reading manifest %s, %v\n", manifestPath, err)
			}
			manifestProgress.Wait()

			var objects []common.ManifestObject
			err = json.Unmarshal(manifestBytes, &objects)
			if err != nil {
				logger.Fatalf("Error has occurred during unmarshalling manifest object: %v\n", err)
			}

			err = download.DownloadMultiple(
				context.Background(),
				dc,
				bk,
				objects,
				downloadPath,
				filenameFormat,
				rename,
				noPrompt,
				protocol,
				numParallel,
				skipCompleted,
			)
			if err != nil {
				logger.Fatal(err.Error())
			}
		},
	}

	downloadMultipleCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	downloadMultipleCmd.MarkFlagRequired("profile") //nolint:errcheck
	downloadMultipleCmd.Flags().StringVar(&manifestPath, "manifest", "", "The manifest file to read from. A valid manifest can be acquired by using the \"Download Manifest\" button in Data Explorer from a data common's portal")
	downloadMultipleCmd.MarkFlagRequired("manifest") //nolint:errcheck
	downloadMultipleCmd.Flags().StringVar(&downloadPath, "download-path", ".", "The directory in which to store the downloaded files")
	downloadMultipleCmd.Flags().StringVar(&filenameFormat, "filename-format", "original", "The format of filename to be used, including \"original\", \"guid\" and \"combined\"")
	downloadMultipleCmd.Flags().BoolVar(&rename, "rename", false, "Only useful when \"--filename-format=original\", will rename file by appending a counter value to its filename if set to true, otherwise the same filename will be used")
	downloadMultipleCmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "If set to true, will not display user prompt message for confirmation")
	downloadMultipleCmd.Flags().StringVar(&protocol, "protocol", "", "Specify the preferred protocol with --protocol=s3")
	downloadMultipleCmd.Flags().IntVar(&numParallel, "numparallel", 1, "Number of downloads to run in parallel")
	downloadMultipleCmd.Flags().BoolVar(&skipCompleted, "skip-completed", false, "If set to true, will check for filename and size before download and skip any files in \"download-path\" that matches both")
	RootCmd.AddCommand(downloadMultipleCmd)
}
