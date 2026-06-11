package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPermissionsCommandRegistersOwnershipAndAccessSubcommands(t *testing.T) {
	arboristCmd := findSubcommand(t, RootCmd, "permissions")
	ownershipCmd := findSubcommand(t, arboristCmd, "ownership")
	accessCmd := findSubcommand(t, arboristCmd, "access")
	findSubcommand(t, arboristCmd, "auth")
	findSubcommand(t, arboristCmd, "org-membership")

	getResourceCmd := findSubcommand(t, ownershipCmd, "get-resource")
	if getResourceCmd.Flags().Lookup("resource") == nil {
		t.Fatal("ownership get-resource missing --resource flag")
	}
	if getResourceCmd.Flags().Lookup("include-children") == nil {
		t.Fatal("ownership get-resource missing --include-children flag")
	}
	if getResourceCmd.Flags().Lookup("include-admins") == nil {
		t.Fatal("ownership get-resource missing --include-admins flag")
	}

	for _, sub := range []string{"grant-user", "revoke-user"} {
		accessUserCmd := findSubcommand(t, accessCmd, sub)
		if accessUserCmd.Flags().Lookup("resource") == nil {
			t.Fatalf("%s missing --resource flag", sub)
		}
		if accessUserCmd.Flags().Lookup("user") == nil {
			t.Fatalf("%s missing --user flag", sub)
		}
		if accessUserCmd.Flags().Lookup("role") == nil {
			t.Fatalf("%s missing --role flag", sub)
		}
	}

	for _, removed := range []string{"policy", "resource", "role", "user"} {
		if hasSubcommand(arboristCmd, removed) {
			t.Fatalf("unexpected legacy subcommand %q still registered", removed)
		}
	}
}

func TestPermissionsRejectsUnknownLegacySubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	RootCmd.SetOut(&stdout)
	RootCmd.SetErr(&stderr)
	RootCmd.SetArgs([]string{"permissions", "policy", "ownership", "--profile", "dev"})

	_, err := RootCmd.ExecuteC()
	if err == nil {
		t.Fatal("expected invalid permissions subcommand to fail")
	}
	if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg(s)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPermissionsLegacyAliasStillWorks(t *testing.T) {
	cmd, _, err := RootCmd.Find([]string{"arborist", "auth", "mapping"})
	if err != nil {
		t.Fatalf("legacy alias lookup failed: %v", err)
	}
	if cmd == nil || cmd.CommandPath() != "calypr-cli permissions auth mapping" {
		t.Fatalf("expected arborist alias to resolve to permissions auth mapping, got %v", cmd)
	}
}

func findSubcommand(t *testing.T, parent *cobra.Command, use string) *cobra.Command {
	t.Helper()
	for _, child := range parent.Commands() {
		if child.Use == use {
			return child
		}
	}
	t.Fatalf("subcommand %q not found", use)
	return nil
}

func hasSubcommand(parent *cobra.Command, use string) bool {
	for _, child := range parent.Commands() {
		if child.Use == use {
			return true
		}
	}
	return false
}
