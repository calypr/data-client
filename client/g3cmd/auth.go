package g3cmd

import (
	"context"
	"encoding/json"
	"log"
	"sort"

	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/calypr/data-client/client/logs"
	"github.com/spf13/cobra"
)

func init() {
	var profile string
	var authCmd = &cobra.Command{
		Use:     "auth",
		Short:   "Return resource access privileges from profile",
		Long:    `Gets resource access privileges for specified profile.`,
		Example: `./data-client auth --profile=<profile-name>`,
		Run: func(cmd *cobra.Command, args []string) {
			// don't initialize transmission logs for non-uploading related commands

			logger, logCloser := logs.New(profile, logs.WithConsole())
			defer logCloser()

			g3i, err := client.NewGen3Interface(context.Background(), profile, logger)
			if err != nil {
				log.Fatalf("Fatal NewGen3Interface error: %s\n", err)
			}

			resourceAccess, err := g3i.CheckPrivileges(g3i.GetCredential())
			if err != nil {
				g3i.Logger().Fatalf("Fatal authentication error: %s\n", err)
			} else {
				if len(resourceAccess) == 0 {
					g3i.Logger().Printf("\nYou don't currently have access to any resources at %s\n", g3i.GetCredential().APIEndpoint)
				} else {
					g3i.Logger().Printf("\nYou have access to the following resource(s) at %s:\n", g3i.GetCredential().APIEndpoint)

					// Sort by resource name
					resources := make([]string, 0, len(resourceAccess))
					for resource := range resourceAccess {
						resources = append(resources, resource)
					}
					sort.Strings(resources)

					for _, project := range resources {
						// Sort by access name if permissions are from Fence
						permissions := resourceAccess[project].([]any)
						_, isFencePermission := permissions[0].(string)
						if isFencePermission {
							access := make([]string, 0, len(permissions))
							for _, permission := range permissions {
								access = append(access, permission.(string))
							}
							sort.Strings(access)
							g3i.Logger().Printf("%s %s\n", project, access)
						} else {
							// Permissions from Arborist already sorted, just pretty print them
							marshalledPermissions, err := json.MarshalIndent(permissions, "", "  ")
							if err != nil {
								g3i.Logger().Printf("%s (error occurred when marshalling permissions): %s\n", project, err)
							}
							g3i.Logger().Printf("%s %s\n", project, marshalledPermissions)
						}
					}
				}
			}
		},
	}

	authCmd.Flags().StringVar(&profile, "profile", "", "Specify the profile to check your access privileges")
	authCmd.MarkFlagRequired("profile") // nolint: errcheck
	RootCmd.AddCommand(authCmd)
}
