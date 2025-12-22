package g3cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/calypr/data-client/client/conf"
	"github.com/calypr/data-client/client/logs"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

var profile string

// Package-level variable to hold the closer function
// (Assuming logs.Closer is a type that can hold a function, like func() error)
var logCloser func()

// Or just:
// var logCloser io.Closer // if closer implements io.Closer

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:     "data-client",
	Short:   "Use the data-client to interact with a Gen3 Data Commons",
	Long:    "Gen3 Client for downloading, uploading and submitting data to data commons.\ndata-client version: " + gitversion + ", commit: " + gitcommit,
	Version: gitversion,
}

// Execute adds all child commands to the root command sets flags appropriately
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if logCloser != nil {
		defer func() {
			logCloser()
		}()
	}

	if err := RootCmd.Execute(); err != nil {
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Define flags and configuration settings.
	RootCmd.PersistentFlags().StringVar(&profile, "profile", "", "Specify profile to use")
	_ = RootCmd.MarkFlagRequired("profile")
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
}

func initConfig() {
	// The logger is needed throughout the application, so we don't store it here,
	// but the closer must be stored.
	logger, closer := logs.New(profile,
		logs.WithConsole(),
		logs.WithMessageFile(),
		logs.WithFailedLog(),
		logs.WithSucceededLog(),
	)

	// 2. ASSIGN CLOSER TO PACKAGE VARIABLE
	logCloser = closer

	// The rest of the function remains the same, except for removing the 'defer resp.Body.Close()'
	// from the initConfig body, as that was unrelated to the logs closer.
	// The rest of your original logic follows...

	conf := conf.Manager{}
	// init local config file
	err := conf.EnsureExists()
	if err != nil {
		logger.Fatal("Error occurred when trying to init config file: " + err.Error())
	}

	// version checker
	if os.Getenv("GEN3_CLIENT_VERSION_CHECK") != "false" &&
		gitversion != "" && gitversion != "N/A" {

		const (
			owner      = "calypr"
			repository = "data-client"
			// The official GitHub API endpoint for the latest release
			apiURL = "https://api.github.com/repos/" + owner + "/" + repository + "/releases/latest"
		)

		client := http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(apiURL)
		if err != nil {
			logger.Println("Error occurred when fetching latest version (HTTP request failed): " + err.Error())
			// Continue execution, as version check failure is non-fatal
			return
		}

		// This defer is correct and should remain, as it cleans up the HTTP response body
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Println("Error occurred when fetching latest version (GitHub API returned status " + strconv.Itoa(resp.StatusCode) + ")")
			return
		}

		var release GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			logger.Println("Error occurred when decoding latest version response: " + err.Error())
			return
		}

		latestVersionTag := release.TagName

		if semver.Compare(gitversion, latestVersionTag) < 0 {
			logger.Println("A new version of data-client is available! The latest version is " + latestVersionTag + ". You are using version " + gitversion)
			logger.Println("Please download the latest data-client release from https://github.com/uc-cdis/cdis-data-client/releases/latest")
		}
	}
}
