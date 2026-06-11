package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var profile string
var backendType string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:           "calypr-cli",
	Short:         "Calypr CLI for data transfer, permissions, collaboration, and portal operations",
	Long:          "Calypr CLI for data transfer, permissions, collaboration, and portal operations.\ncalypr-cli version: " + gitversion + ", commit: " + gitcommit,
	Version:       gitversion,
	SilenceErrors: true,
}

// Execute adds all child commands to the root command sets flags appropriately
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVar(&profile, "profile", "", "Specify profile to use")
	RootCmd.PersistentFlags().StringVar(&backendType, "backend", "gen3", "Specify backend to use (gen3 or drs)")
	_ = RootCmd.MarkFlagRequired("profile")
}
