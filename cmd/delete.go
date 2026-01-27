package cmd

import (
	"context"

	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	"github.com/spf13/cobra"
)

//Not support yet, place holder only

func init() {
	var guid string
	var deleteCmd = &cobra.Command{ // nolint:deadcode,unused,varcheck
		Use:   "delete",
		Short: "Send DELETE HTTP Request for given URI",
		Long: `Deletes a given URI from the database.
If no profile is specified, "default" profile is used for authentication.`,
		Example: `./data-client delete --uri=v0/submission/bpa/test/entities/example_id
	  ./data-client delete --profile=user1 --uri=v0/submission/bpa/test/entities/1af1d0ab-efec-4049-98f0-ae0f4bb1bc64`,
		Run: func(cmd *cobra.Command, args []string) {

			logger, logCloser := logs.New(profile, logs.WithConsole())
			defer logCloser()

			g3i, err := client.NewGen3Interface(profile, logger)
			if err != nil {
				logger.Fatalf("Fatal NewGen3Interface error: %s\n", err)
			}

			msg, err := g3i.Fence().DeleteRecord(context.Background(), guid)
			if err != nil {
				logger.Fatal(err)
			}
			logger.Println(msg)
		},
	}

	deleteCmd.Flags().StringVar(&profile, "guid", "", "Specify the profile to check your access privileges")
	RootCmd.AddCommand(deleteCmd)
}
