package g3cmd

import (
	"context"
	"encoding/json"
	"log"
	"sort"

	client "github.com/calypr/data-client/client/gen3Client"
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
			gen3Interface, err := client.NewGen3Interface(context.Background(), profile)
			if err != nil {
				log.Fatalf("Fatal NewGen3Interface error: %s\n", err)
			}
			host, resourceAccess, err := gen3Interface.CheckPrivileges()
			if err != nil {
				log.Fatalf("Fatal authentication error: %s\n", err)
			} else {
				if len(resourceAccess) == 0 {
					log.Printf("\nYou don't currently have access to any resources at %s\n", host)
				} else {
					log.Printf("\nYou have access to the following resource(s) at %s:\n", host)

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
							log.Printf("%s %s\n", project, access)
						} else {
							// Permissions from Arborist already sorted, just pretty print them
							marshalledPermissions, err := json.MarshalIndent(permissions, "", "  ")
							if err != nil {
								log.Printf("%s (error occurred when marshalling permissions): %s\n", project, err)
							}
							log.Printf("%s %s\n", project, marshalledPermissions)
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
