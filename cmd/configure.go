package cmd

import (
	"context"
	"fmt"

	"github.com/calypr/calypr-cli/conf"
	"github.com/calypr/calypr-cli/g3client"
	"github.com/calypr/calypr-cli/logs"
	"github.com/spf13/cobra"
)

func mergeImportedCredential(target *conf.Credential, imported *conf.Credential) {
	if target == nil || imported == nil {
		return
	}
	target.KeyID = imported.KeyID
	target.APIKey = imported.APIKey
	if target.APIEndpoint == "" && imported.APIEndpoint != "" {
		target.APIEndpoint = imported.APIEndpoint
	}
	target.AccessToken = ""
}

func init() {
	var profile string
	var credFile string
	var fenceToken string
	var apiEndpoint string
	var useShepherd string
	var minShepherdVersion string
	var configureCmd = &cobra.Command{
		Use:   "configure",
		Short: "Add or modify a configuration profile to your config file",
		Long: `Configuration file located at ~/.gen3/gen3_client_config.ini
	If a field is left empty, the existing value (if it exists) will remain unchanged`,
		Example: `./calypr-cli configure --profile=<profile-name> --cred=<path-to-credential/cred.json> --apiendpoint=https://data.mycommons.org`,
		Run: func(cmd *cobra.Command, args []string) {
			// don't initialize transmission logs for non-uploading related commands
			cred := &conf.Credential{
				Profile:            profile,
				APIEndpoint:        apiEndpoint,
				AccessToken:        fenceToken,
				UseShepherd:        useShepherd,
				MinShepherdVersion: minShepherdVersion,
			}
			logger, logCloser := logs.New(profile, logs.WithConsole())
			defer logCloser()

			configure := conf.NewConfigure(logger.Logger)
			if credFile != "" {
				readCred, err := configure.Import(credFile, "")
				if err != nil {
					logger.Fatal(err) // or return proper error
				}
				mergeImportedCredential(cred, readCred)
			}

			g3i := g3client.NewGen3InterfaceFromCredential(cred, logger, g3client.WithClients())
			err := g3i.Credentials().Export(context.Background(), cred)
			if err != nil {
				logger.Println(err.Error())
			}
			logger.Println(`Profile '` + profile + `' has been configured successfully.`)
		},
	}

	configureCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	configureCmd.MarkFlagRequired("profile") //nolint:errcheck
	configureCmd.Flags().StringVar(&credFile, "cred", "", "Specify the credential file that you want to use")
	configureCmd.MarkFlagRequired("cred") //nolint:errcheck
	configureCmd.Flags().StringVar(&fenceToken, "fenceToken", "", "Specify the fence token to use as a substitute for credential file")
	configureCmd.Flags().StringVar(&apiEndpoint, "apiendpoint", "", "Specify the API endpoint of the data commons")
	configureCmd.MarkFlagRequired("apiendpoint") //nolint:errcheck
	configureCmd.Flags().StringVar(&useShepherd, "use-shepherd", "", fmt.Sprintf("Enables or disables support for the Shepherd API. If enabled, gen3client will use the Shepherd API if available. (Default: %v)", false))
	configureCmd.Flags().StringVar(&minShepherdVersion, "min-shepherd-version", "", fmt.Sprintf("Specify the minimum version of Shepherd that the gen3client will use if Shepherd is enabled. (Default: %v)", "2.0.0"))
	RootCmd.AddCommand(configureCmd)
}
