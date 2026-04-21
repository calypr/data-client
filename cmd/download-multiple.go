package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	sydownload "github.com/calypr/syfon/client/xfer/download"
	"github.com/spf13/cobra"
)

func init() {
	var manifestPath string
	var downloadPath string
	var numParallel int
	var profile string

	var downloadMultipleCmd = &cobra.Command{
		Use:     "download-multiple",
		Short:   "Download multiple of files from a specified manifest",
		Long:    `Get presigned URLs for multiple of files specified in a manifest file and then download all of them.`,
		Example: `./data-client download-multiple --profile <profile-name> --manifest <path-to-manifest/manifest.json> --download-path <path-to-file-dir/>`,
		Run: func(cmd *cobra.Command, args []string) {
			logger, logCloser := logs.New(profile, logs.WithConsole(), logs.WithFailedLog(), logs.WithScoreboard(), logs.WithSucceededLog())
			defer logCloser()

			g3i, err := g3client.NewGen3Interface(profile, logger)
			if err != nil {
				log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
			}

			guids, err := loadManifestGuids(manifestPath)
			if err != nil {
				log.Fatalf("Failed to read manifest %s: %v", manifestPath, err)
			}

			syfon := g3i.SyfonClient()
			if syfon == nil {
				logger.Fatal("failed to initialize syfon client")
			}
			err = sydownload.DownloadMultiple(context.Background(), syfon.DRS(), syfon.Data(), guids, downloadPath, numParallel, false)
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
	downloadMultipleCmd.Flags().IntVar(&numParallel, "numparallel", 1, "Number of downloads to run in parallel")
	RootCmd.AddCommand(downloadMultipleCmd)
}

func loadManifestGuids(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entries []struct {
		GUID string `json:"guid"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	guids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.GUID != "" {
			guids = append(guids, entry.GUID)
		}
	}
	if len(guids) == 0 {
		return nil, fmt.Errorf("manifest %s did not contain any GUIDs", path)
	}
	return guids, nil
}
