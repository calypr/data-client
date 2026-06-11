package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestGeckoCommandRegistersExpectedSubcommands(t *testing.T) {
	geckoCmd := findSubcommand(t, RootCmd, "gecko")
	findSubcommand(t, geckoCmd, "health")

	configCmd := findSubcommand(t, geckoCmd, "config")
	findSubcommand(t, configCmd, "types")
	findSubcommandPrefix(t, configCmd, "list")
	findSubcommandPrefix(t, configCmd, "get")

	projectCmd := findSubcommand(t, geckoCmd, "project")
	findSubcommandPrefix(t, projectCmd, "get")
	putProjectCmd := findSubcommandPrefix(t, projectCmd, "put")
	findSubcommandPrefix(t, projectCmd, "delete")
	if putProjectCmd.Flags().Lookup("file") == nil {
		t.Fatal("gecko project put missing --file flag")
	}

	appCardCmd := findSubcommand(t, geckoCmd, "appcard")
	findSubcommandPrefix(t, appCardCmd, "get")
	putAppCardCmd := findSubcommandPrefix(t, appCardCmd, "put")
	findSubcommandPrefix(t, appCardCmd, "delete")
	if putAppCardCmd.Flags().Lookup("file") == nil {
		t.Fatal("gecko appcard put missing --file flag")
	}
}

func findSubcommandPrefix(t *testing.T, parent interface{ Commands() []*cobra.Command }, prefix string) *cobra.Command {
	t.Helper()
	for _, child := range parent.Commands() {
		if strings.HasPrefix(child.Use, prefix+" ") || child.Use == prefix {
			return child
		}
	}
	t.Fatalf("subcommand prefix %q not found", prefix)
	return nil
}
