package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/calypr/calypr-cli/arborist"
	"github.com/calypr/calypr-cli/g3client"
	"github.com/calypr/calypr-cli/logs"
	"github.com/spf13/cobra"
)

type arboristCommandOptions struct {
	jsonOutput bool
}

var arboristOpts arboristCommandOptions

var arboristCmd = &cobra.Command{
	Use:     "permissions",
	Aliases: []string{"arborist"},
	Short:   "Manage permissions, ownership, and organization access",
	Long: "Permissions and ownership power tools. These commands use the Arborist routes exposed through " +
		"the configured Gen3 profile.",
	RunE: groupHelpOrUnknownError,
}

func init() {
	arboristCmd.PersistentFlags().BoolVar(&arboristOpts.jsonOutput, "json", false, "Output raw JSON responses")
	RootCmd.AddCommand(arboristCmd)

	registerArboristAuthCommands()
	registerArboristOwnershipCommands()
	registerArboristAccessCommands()
	registerArboristOrgMembershipCommands()
}

func getArboristClient() (arborist.ClientInterface, func(), error) {
	if strings.TrimSpace(profile) == "" {
		return nil, nil, fmt.Errorf("profile is required; use --profile")
	}
	logger, logCloser := logs.New(profile)
	g3i, err := g3client.NewGen3Interface(profile, logger)
	if err != nil {
		logCloser()
		return nil, nil, fmt.Errorf("access Gen3: %w", err)
	}
	return arborist.NewClient(g3i, g3i.Credentials().Current()), logCloser, nil
}

func runArborist(cmd *cobra.Command, action func(context.Context, arborist.ClientInterface) (any, string, error)) error {
	client, closer, err := getArboristClient()
	if err != nil {
		return err
	}
	defer closer()

	result, message, err := action(cmd.Context(), client)
	if err != nil {
		return err
	}
	if arboristOpts.jsonOutput {
		if result == nil {
			result = map[string]string{"message": message}
		}
		return writeJSON(cmd, result)
	}
	if strings.TrimSpace(message) != "" {
		fmt.Fprintln(cmd.OutOrStdout(), message)
		return nil
	}
	return writeJSON(cmd, result)
}

func writeJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func validEmailArg(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !emailRegex.MatchString(value) {
		return "", fmt.Errorf("invalid email address %q", value)
	}
	return value, nil
}

func groupHelpOrUnknownError(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	cmd.SilenceUsage = true
	return fmt.Errorf("unknown command %q for %q", strings.Join(args, " "), cmd.CommandPath())
}

func registerArboristAuthCommands() {
	authCmd := &cobra.Command{Use: "auth", Short: "Inspect current authorization mapping", RunE: groupHelpOrUnknownError}
	authCmd.AddCommand(&cobra.Command{
		Use:   "mapping",
		Short: "Get auth mapping for the current token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runArborist(cmd, func(ctx context.Context, client arborist.ClientInterface) (any, string, error) {
				var out any
				err := client.AuthMapping(ctx, &out)
				return out, "", err
			})
		},
	})
	arboristCmd.AddCommand(authCmd)
}

func registerArboristOwnershipCommands() {
	ownershipCmd := &cobra.Command{Use: "ownership", Short: "Manage ownership-backed access state", RunE: groupHelpOrUnknownError}
	var parentPath, childName, templateName, description string
	createDescendantCmd := &cobra.Command{
		Use:   "create-descendant",
		Short: "Create a missing child resource and owner grants",
		RunE: func(cmd *cobra.Command, args []string) error {
			if parentPath == "" || childName == "" {
				return fmt.Errorf("--parent and --name are required")
			}
			return runArborist(cmd, func(ctx context.Context, client arborist.ClientInterface) (any, string, error) {
				var out any
				err := client.CreateOwnedDescendant(ctx, arborist.CreateOwnedDescendantRequest{
					ParentPath:  parentPath,
					Name:        childName,
					Template:    templateName,
					Description: description,
				}, &out)
				return out, fmt.Sprintf("Created owned descendant %s under %s", childName, parentPath), err
			})
		},
	}
	createDescendantCmd.Flags().StringVar(&parentPath, "parent", "", "Parent resource path")
	createDescendantCmd.Flags().StringVar(&childName, "name", "", "Child resource name")
	createDescendantCmd.Flags().StringVar(&templateName, "template", "", "Ownership template")
	createDescendantCmd.Flags().StringVar(&description, "description", "", "Resource description")
	ownershipCmd.AddCommand(createDescendantCmd)

	ownershipCmd.AddCommand(ownerMutationCmd("add-owner", "Add an owner", true))
	ownershipCmd.AddCommand(ownerMutationCmd("rm-owner", "Remove an owner", false))
	ownershipCmd.AddCommand(ownershipReadCmd())
	arboristCmd.AddCommand(ownershipCmd)
}

func registerArboristAccessCommands() {
	accessCmd := &cobra.Command{
		Use:   "access",
		Short: "Manage direct non-owner access grants",
		RunE:  groupHelpOrUnknownError,
	}
	accessCmd.AddCommand(accessUserMutationCmd("grant-user", "Grant direct user access on a resource", true))
	accessCmd.AddCommand(accessUserMutationCmd("revoke-user", "Revoke direct user access from a resource", false))
	arboristCmd.AddCommand(accessCmd)
}

func registerArboristOrgMembershipCommands() {
	orgCmd := &cobra.Command{
		Use:   "org-membership",
		Short: "Manage organization membership grants",
		Long: "Convenience wrapper around direct access grants. The default role is org-member, which grants " +
			"project creation under the organization without granting ownership over existing projects.",
		RunE: groupHelpOrUnknownError,
	}
	orgCmd.AddCommand(orgMembershipMutationCmd("add [email] [organization]", "Grant organization membership", true))
	orgCmd.AddCommand(orgMembershipMutationCmd("rm [email] [organization]", "Remove organization membership", false))
	arboristCmd.AddCommand(orgCmd)
}

func ownerMutationCmd(use string, short string, add bool) *cobra.Command {
	var resourcePath, username string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if resourcePath == "" || username == "" {
				return fmt.Errorf("--resource and --user are required")
			}
			username, err := validEmailArg(username)
			if err != nil {
				return err
			}
			return runArborist(cmd, func(ctx context.Context, client arborist.ClientInterface) (any, string, error) {
				req := arborist.OwnerMutationRequest{ResourcePath: resourcePath, Username: username}
				if add {
					var out any
					err := client.AddOwner(ctx, req, &out)
					return out, fmt.Sprintf("Added owner %s on %s", username, resourcePath), err
				}
				return nil, fmt.Sprintf("Removed owner %s from %s", username, resourcePath), client.RemoveOwner(ctx, req)
			})
		},
	}
	cmd.Flags().StringVar(&resourcePath, "resource", "", "Owned resource path")
	cmd.Flags().StringVar(&username, "user", "", "Username")
	return cmd
}

func ownershipReadCmd() *cobra.Command {
	var resourcePath string
	var includeChildren bool
	var includeAdmins bool
	cmd := &cobra.Command{
		Use:   "get-resource",
		Short: "Read ownership and direct-access state for a resource",
		RunE: func(cmd *cobra.Command, args []string) error {
			if resourcePath == "" {
				return fmt.Errorf("--resource is required")
			}
			return runArborist(cmd, func(ctx context.Context, client arborist.ClientInterface) (any, string, error) {
				var out any
				err := client.GetOwnershipResource(ctx, resourcePath, includeChildren, includeAdmins, &out)
				return out, "", err
			})
		},
	}
	cmd.Flags().StringVar(&resourcePath, "resource", "", "Resource path to inspect")
	cmd.Flags().BoolVar(&includeChildren, "include-children", false, "Include descendant resources in the ownership view")
	cmd.Flags().BoolVar(&includeAdmins, "include-admins", false, "Include protected admin rows in the ownership view")
	return cmd
}

func accessUserMutationCmd(use string, short string, grant bool) *cobra.Command {
	var resourcePath, username, roleID string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if resourcePath == "" || username == "" || roleID == "" {
				return fmt.Errorf("--resource, --user, and --role are required")
			}
			username, err := validEmailArg(username)
			if err != nil {
				return err
			}
			return runArborist(cmd, func(ctx context.Context, client arborist.ClientInterface) (any, string, error) {
				req := arborist.AccessUserRequest{ResourcePath: resourcePath, Username: username, RoleID: roleID}
				if grant {
					var out any
					err := client.GrantAccessUser(ctx, req, &out)
					return out, fmt.Sprintf("Granted direct access role %s to %s on %s", roleID, username, resourcePath), err
				}
				return nil, fmt.Sprintf("Revoked direct access role %s from %s on %s", roleID, username, resourcePath), client.RevokeAccessUser(ctx, req)
			})
		},
	}
	cmd.Flags().StringVar(&resourcePath, "resource", "", "Resource path")
	cmd.Flags().StringVar(&username, "user", "", "Username")
	cmd.Flags().StringVar(&roleID, "role", "", "Role ID")
	return cmd
}

func orgMembershipMutationCmd(use string, short string, grant bool) *cobra.Command {
	var roleID string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			username, err := validEmailArg(args[0])
			if err != nil {
				return err
			}
			organization := strings.TrimSpace(args[1])
			return runArborist(cmd, func(ctx context.Context, client arborist.ClientInterface) (any, string, error) {
				role := roleIDOrDefault(roleID)
				if grant {
					err := client.GrantOrgMembership(ctx, organization, username, roleID)
					return nil, fmt.Sprintf("Granted %s role %s on /programs/%s", username, role, organization), err
				}
				err := client.RevokeOrgMembership(ctx, organization, username, roleID)
				return nil, fmt.Sprintf("Removed %s role %s from /programs/%s", username, role, organization), err
			})
		},
	}
	cmd.Flags().StringVar(&roleID, "role", arborist.DefaultOrgMemberRole, "Arborist role to grant on the organization projects container")
	return cmd
}

func roleIDOrDefault(roleID string) string {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return arborist.DefaultOrgMemberRole
	}
	return roleID
}
