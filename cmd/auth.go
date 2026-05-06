package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	"github.com/spf13/cobra"
)

const authSummaryResourceLimit = 10

func init() {
	var profile string
	var showAll bool
	var jsonOutput bool
	var authCmd = &cobra.Command{
		Use:   "auth",
		Short: "Return resource access privileges from profile",
		Long:  `Gets resource access privileges for specified profile.`,
		Example: `./data-client auth --profile=<profile-name>
./data-client auth --profile=<profile-name> --all
./data-client auth --profile=<profile-name> --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// don't initialize transmission logs for non-uploading related commands

			logger, logCloser := logs.New(profile, logs.WithNoConsole())
			defer logCloser()

			g3i, err := g3client.NewGen3Interface(
				profile, logger,
				g3client.WithClients(g3client.FenceClient),
			)
			if err != nil {
				return fmt.Errorf("new Gen3 interface: %w", err)
			}

			resourceAccess, err := g3i.FenceClient().CheckPrivileges(context.Background())
			if err != nil {
				return fmt.Errorf("authentication: %w", err)
			}

			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(resourceAccess)
			}

			writeAuthSummary(cmd.OutOrStdout(), g3i.Credentials().Current().APIEndpoint, resourceAccess, showAll)
			return nil
		},
	}

	authCmd.Flags().StringVar(&profile, "profile", "", "Specify the profile to check your access privileges")
	authCmd.Flags().BoolVar(&showAll, "all", false, "Show every resource in each permission group")
	authCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output raw resource access JSON")
	authCmd.MarkFlagRequired("profile") // nolint: errcheck
	RootCmd.AddCommand(authCmd)
}

func writeAuthSummary(w io.Writer, endpoint string, resourceAccess map[string]any, showAll bool) {
	if len(resourceAccess) == 0 {
		fmt.Fprintf(w, "No resource access found for %s\n", endpoint)
		return
	}

	groups := make(map[string][]string)
	for resource, permissions := range resourceAccess {
		signature := authPermissionSignature(permissions)
		groups[signature] = append(groups[signature], resource)
	}

	type accessGroup struct {
		signature string
		resources []string
	}

	orderedGroups := make([]accessGroup, 0, len(groups))
	for signature, resources := range groups {
		sort.Strings(resources)
		orderedGroups = append(orderedGroups, accessGroup{
			signature: signature,
			resources: resources,
		})
	}

	sort.Slice(orderedGroups, func(i, j int) bool {
		if len(orderedGroups[i].resources) != len(orderedGroups[j].resources) {
			return len(orderedGroups[i].resources) > len(orderedGroups[j].resources)
		}
		return orderedGroups[i].signature < orderedGroups[j].signature
	})

	fmt.Fprintf(w, "Access for %s\n", endpoint)
	fmt.Fprintf(w, "%d resources in %d permission groups\n\n", len(resourceAccess), len(orderedGroups))

	for _, group := range orderedGroups {
		fmt.Fprintf(w, "%d %s: %s\n", len(group.resources), pluralize("resource", len(group.resources)), group.signature)

		limit := len(group.resources)
		if !showAll && limit > authSummaryResourceLimit {
			limit = authSummaryResourceLimit
		}

		for _, resource := range group.resources[:limit] {
			fmt.Fprintf(w, "  %s\n", resource)
		}
		if !showAll && len(group.resources) > limit {
			fmt.Fprintf(w, "  ... %d more (use --all to show every resource)\n", len(group.resources)-limit)
		}
		fmt.Fprintln(w)
	}
}

func pluralize(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}

func authPermissionSignature(value any) string {
	permissions, ok := value.([]any)
	if !ok {
		return compactJSON(value)
	}
	if len(permissions) == 0 {
		return "no permissions"
	}

	if _, ok := permissions[0].(string); ok {
		access := make([]string, 0, len(permissions))
		for _, permission := range permissions {
			access = append(access, fmt.Sprint(permission))
		}
		sort.Strings(access)
		return strings.Join(access, ", ")
	}

	serviceMethods := make(map[string]map[string]struct{})
	unknown := make([]string, 0)

	for _, permission := range permissions {
		method, service, ok := authPermissionFields(permission)
		if !ok {
			unknown = append(unknown, compactJSON(permission))
			continue
		}

		if serviceMethods[service] == nil {
			serviceMethods[service] = make(map[string]struct{})
		}
		serviceMethods[service][method] = struct{}{}
	}

	parts := make([]string, 0, len(serviceMethods)+len(unknown))
	services := make([]string, 0, len(serviceMethods))
	for service := range serviceMethods {
		services = append(services, service)
	}
	sort.Strings(services)

	for _, service := range services {
		methods := make([]string, 0, len(serviceMethods[service]))
		for method := range serviceMethods[service] {
			methods = append(methods, method)
		}
		sort.Strings(methods)
		parts = append(parts, fmt.Sprintf("%s: %s", service, strings.Join(methods, ", ")))
	}

	sort.Strings(unknown)
	parts = append(parts, unknown...)
	return strings.Join(parts, "; ")
}

func authPermissionFields(value any) (method string, service string, ok bool) {
	switch permission := value.(type) {
	case map[string]any:
		method, methodOK := permission["method"].(string)
		service, serviceOK := permission["service"].(string)
		return method, service, methodOK && serviceOK
	case map[string]string:
		method, methodOK := permission["method"]
		service, serviceOK := permission["service"]
		return method, service, methodOK && serviceOK
	default:
		return "", "", false
	}
}

func compactJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}
