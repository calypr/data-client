package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/gecko"
	"github.com/calypr/data-client/logs"
	"github.com/spf13/cobra"
)

type geckoCommandOptions struct {
	jsonOutput bool
}

var geckoOpts geckoCommandOptions

var geckoCmd = &cobra.Command{
	Use:   "gecko",
	Short: "Gecko configuration power tools",
	Long:  "Gecko configuration power tools for health checks, typed config inspection, project configs, and app cards.",
	RunE:  groupHelpOrUnknownError,
}

func init() {
	geckoCmd.PersistentFlags().BoolVar(&geckoOpts.jsonOutput, "json", false, "Output raw JSON responses")
	RootCmd.AddCommand(geckoCmd)

	registerGeckoHealthCommands()
	registerGeckoConfigCommands()
	registerGeckoProjectCommands()
	registerGeckoAppCardCommands()
}

func getGeckoClient() (gecko.GeckoInterface, func(), error) {
	if strings.TrimSpace(profile) == "" {
		return nil, nil, fmt.Errorf("profile is required; use --profile")
	}
	logger, logCloser := logs.New(profile)
	g3i, err := g3client.NewGen3Interface(profile, logger)
	if err != nil {
		logCloser()
		return nil, nil, fmt.Errorf("access Gen3: %w", err)
	}
	return g3i.GeckoClient(), logCloser, nil
}

func runGecko(cmd *cobra.Command, action func(context.Context, gecko.GeckoInterface) (any, string, error)) error {
	client, closer, err := getGeckoClient()
	if err != nil {
		return err
	}
	defer closer()

	result, message, err := action(cmd.Context(), client)
	if err != nil {
		return err
	}
	if geckoOpts.jsonOutput {
		if result == nil {
			result = map[string]string{"message": message}
		}
		return writeGeckoJSON(cmd, result)
	}
	if strings.TrimSpace(message) != "" {
		fmt.Fprintln(cmd.OutOrStdout(), message)
		return nil
	}
	return writeGeckoJSON(cmd, result)
}

func writeGeckoJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func readJSONFile[T any](path string) (*T, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("--file is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func parseConfigType(raw string) (gecko.ConfigType, error) {
	configType := gecko.ConfigType(strings.TrimSpace(raw))
	for _, known := range gecko.KnownConfigTypes() {
		if configType == known {
			return configType, nil
		}
	}
	return "", fmt.Errorf("unknown Gecko config type %q", raw)
}

func registerGeckoHealthCommands() {
	geckoCmd.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Check Gecko health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				health, err := client.HealthCheck(ctx)
				if err != nil {
					return nil, "", err
				}
				return map[string]string{"status": health}, "", nil
			})
		},
	})
}

func registerGeckoConfigCommands() {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect Gecko config types and config IDs",
		RunE:  groupHelpOrUnknownError,
	}
	configCmd.AddCommand(&cobra.Command{
		Use:   "types",
		Short: "List known Gecko config types",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				types, err := client.ListConfigTypes(ctx)
				if err != nil {
					return nil, "", err
				}
				return types, "", nil
			})
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "list [type]",
		Short: "List config IDs for a Gecko config type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configType, err := parseConfigType(args[0])
			if err != nil {
				return err
			}
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				configs, err := client.ListConfigs(ctx, configType)
				if err != nil {
					return nil, "", err
				}
				return configs, "", nil
			})
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "get [type] [id]",
		Short: "Get a Gecko config by type and ID",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			configType, err := parseConfigType(args[0])
			if err != nil {
				return err
			}
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				var out any
				err := client.GetConfig(ctx, configType, args[1], &out)
				return out, "", err
			})
		},
	})
	geckoCmd.AddCommand(configCmd)
}

func registerGeckoProjectCommands() {
	projectCmd := &cobra.Command{
		Use:   "project",
		Short: "Manage Gecko project configs",
		RunE:  groupHelpOrUnknownError,
	}
	projectCmd.AddCommand(&cobra.Command{
		Use:   "get [id]",
		Short: "Get a Gecko project config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				cfg, err := gecko.GetProjectConfig(ctx, client, args[0])
				return cfg, "", err
			})
		},
	})

	var projectFile string
	putProjectCmd := &cobra.Command{
		Use:   "put [id]",
		Short: "Create or replace a Gecko project config from JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				cfg, err := readJSONFile[gecko.ProjectConfig](projectFile)
				if err != nil {
					return nil, "", err
				}
				status, err := gecko.PutProjectConfig(ctx, client, args[0], *cfg)
				return status, fmt.Sprintf("Upserted Gecko project config %s", args[0]), err
			})
		},
	}
	putProjectCmd.Flags().StringVar(&projectFile, "file", "", "JSON file containing Gecko project config")
	projectCmd.AddCommand(putProjectCmd)

	projectCmd.AddCommand(&cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a Gecko project config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				status, err := gecko.DeleteProjectConfig(ctx, client, args[0])
				return status, fmt.Sprintf("Deleted Gecko project config %s", args[0]), err
			})
		},
	})
	geckoCmd.AddCommand(projectCmd)
}

func registerGeckoAppCardCommands() {
	appCardCmd := &cobra.Command{
		Use:   "appcard",
		Short: "Manage Gecko app cards",
		RunE:  groupHelpOrUnknownError,
	}
	appCardCmd.AddCommand(&cobra.Command{
		Use:   "get [project-id]",
		Short: "Get a Gecko app card",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				card, err := gecko.GetAppCard(ctx, client, args[0])
				return card, "", err
			})
		},
	})

	var appCardFile string
	putAppCardCmd := &cobra.Command{
		Use:   "put [project-id]",
		Short: "Create or replace a Gecko app card from JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				card, err := readJSONFile[gecko.AppCard](appCardFile)
				if err != nil {
					return nil, "", err
				}
				status, err := gecko.UpsertAppCard(ctx, client, args[0], *card)
				return status, fmt.Sprintf("Upserted Gecko app card %s", args[0]), err
			})
		},
	}
	putAppCardCmd.Flags().StringVar(&appCardFile, "file", "", "JSON file containing Gecko app card")
	appCardCmd.AddCommand(putAppCardCmd)

	appCardCmd.AddCommand(&cobra.Command{
		Use:   "delete [project-id]",
		Short: "Delete a Gecko app card",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGecko(cmd, func(ctx context.Context, client gecko.GeckoInterface) (any, string, error) {
				status, err := gecko.DeleteAppCard(ctx, client, args[0])
				return status, fmt.Sprintf("Deleted Gecko app card %s", args[0]), err
			})
		},
	})
	geckoCmd.AddCommand(appCardCmd)
}
